package file_service

import (
	"better_mp3/app/logger"
	"better_mp3/app/member_service"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"strings"
	"time"
)

var promptChannel = make(chan string)
var MyHash uint32

type FileServer struct {
	selfIP     string
	MemberInfo member_service.MemberServer
	FileTable  FileTable
	config     FileServiceConfig
}

func NewFileServer(memberService member_service.MemberServer) FileServer {
	var fs FileServer
	fs.config = GetFileServiceConfig()
	fs.selfIP = member_service.FindLocalhostIp()
	if fs.selfIP == "" {
		log.Fatal("ERROR get localhost IP")
	}
	fs.FileTable = NewFileTable(&fs)
	fs.MemberInfo = memberService
	go fs.FileTable.RunDaemon(
		fs.MemberInfo.JoinedNodeChan,
		fs.MemberInfo.LeftNodesChan)
	return fs
}

func (fs *FileServer) Run() {
	go RunRPCServer(fs)
	logger.PrintInfo(
		"File Service is now running on port " + fs.config.Port,
		"\n\tSDFS file path: ", fs.config.Path)
}

func (fs *FileServer) LocalRep(filename string, success *bool) error {
	var content string
	locations := fs.FileTable.ListLocations(filename)
	if len(locations) == 0 {
		return errors.New("no replica available")
	} else {
		for _, ip := range locations {
			var buffer []byte
			if ip == fs.selfIP {
				err := fs.LocalGet(filename, &buffer)
				if err != nil {
					continue
				}
			} else {
				client, err := rpc.Dial("tcp", ip+":"+fs.config.Port)
				if err != nil {
					continue
				}
				err = client.Call("FileRPCServer.LocalGet", filename, &buffer)
				if err != nil {
					continue
				}
			}
			content = string(buffer)
		}
	}
	err := fs.LocalPut(map[string]string{"filename": filename, "content": content}, success)
	return err
}

func (fs *FileServer) LocalPut(args map[string]string, success *bool) error {
	err := ioutil.WriteFile(fs.config.Path+args["filename"], []byte(args["content"]), os.ModePerm)
	return err
}

func (fs *FileServer) LocalAppend(args map[string]string, success *bool) error {
	f, err := os.OpenFile(fs.config.Path+args["filename"], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(args["content"])); err != nil {
		return err
	}
	err = f.Close()
	return err
}

func (fs *FileServer) confirm(local string, remote string) {
	buf := bufio.NewReader(os.Stdin)
	go func() {
		time.Sleep(time.Second * 30)
		promptChannel <- "ok"
	}()
	for {
		select {
		case <-promptChannel:
			fmt.Println("Timeout")
			return
		default:
			sentence, err := buf.ReadBytes('\n')
			cmd := strings.Split(string(bytes.Trim([]byte(sentence), "\n")), " ")
			if err == nil && len(cmd) == 1 {
				if cmd[0] == "y" || cmd[0] == "yes" {
					fs.Put(local, remote)
				} else if cmd[0] == "n" || cmd[0] == "no" {
					return
				}
			}
		}
	}
}

func (fs *FileServer) TemptPut(local string, remote string) {
	_, ok := fs.FileTable.latest[remote]
	if ok && time.Now().UnixNano()-fs.FileTable.latest[remote] < int64(time.Minute) {
		fmt.Println("Confirm update? (y/n)")
		fs.confirm(local, remote)
	} else {
		fs.Put(local, remote)
	}
}

// local: local file name
// remote: remote file name
func (fs *FileServer) Put(local string, remote string) {
	target_ips := fs.FileTable.search(remote)
	//fmt.Println(target_ips)
	for _, ip := range target_ips {
		content, err := ioutil.ReadFile(local)
		if err != nil {
			fmt.Println("Local file", local, "doesn't exist!")
			return
		} else {
			client, err := rpc.Dial("tcp", ip+":"+fs.config.Port)
			if err != nil {
				log.Println(err)
				continue
			}
			var success bool
			err = client.Call("FileRPCServer.LocalPut", map[string]string{
				"filename": remote,
				"content":  string(content),
			}, &success)
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}
	var success bool
	err := fs.FileTable.PutEntry(remote, &success)
	if err != nil {
		log.Println(err)
	}
	for id, _ := range fs.MemberInfo.Members {
		client, err := rpc.Dial("tcp", strings.Split(id, "_")[0]+":"+fs.config.Port)
		if err != nil {
			log.Println(err)
			continue
		}
		err = client.Call("FileRPCServer.PutEntry", remote, &success)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}

func (fs *FileServer) LocalGet(filename string, content *[]byte) error {
	var err error
	*content, err = ioutil.ReadFile(fs.config.Path + filename)
	return err
}

func (fs *FileServer) Get(sdfs string, local string) {
	locations := fs.FileTable.ListLocations(sdfs)
	if len(locations) == 0 {
		fmt.Println("The file is not available!")
	} else {
		for _, ip := range locations {
			var buffer []byte
			if ip == fs.selfIP {
				err := fs.LocalGet(sdfs, &buffer)
				if err != nil {
					continue
				}
			} else {
				client, err := rpc.Dial("tcp", ip+":"+fs.config.Port)
				if err != nil {
					continue
				}
				err = client.Call("FileRPCServer.LocalGet", sdfs, &buffer)
				if err != nil {
					continue
				}
			}
			err := ioutil.WriteFile(local, buffer, os.ModePerm)
			if err != nil {
				continue
			}
			break
		}
	}
}

func (fs *FileServer) LocalDel(filename string, success *bool) error {
	err := os.Remove(fs.config.Path + filename)
	return err
}

func (fs *FileServer) Delete(sdfs string) {
	locations := fs.FileTable.ListLocations(sdfs)
	if len(locations) == 0 {
		fmt.Println("The file is not available!")
	} else {
		//fmt.Println(locations)
		var success bool
		for _, ip := range locations {
			if ip == fs.selfIP {
				err := fs.LocalDel(sdfs, &success)
				if err != nil {
					log.Println(err)
				}
			} else {
				client, err := rpc.Dial("tcp", ip+":"+fs.config.Port)
				if err != nil {
					log.Println(err)
					continue
				}
				err = client.Call("FileRPCServer.LocalDel", sdfs, &success)
				if err != nil {
					log.Println(err)
					continue
				}
			}
		}
		err := fs.FileTable.DeleteEntry(sdfs, &success)
		if err != nil {
			log.Println(err)
		}
		for id, _ := range fs.MemberInfo.Members {
			client, err := rpc.Dial("tcp", strings.Split(id, "_")[0]+":"+fs.config.Port)
			if err != nil {
				log.Println(err)
				continue
			}
			err = client.Call("FileRPCServer.DelEntry", sdfs, &success)
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}
}

func (fs *FileServer) Append(content string, remote string) {
	target_ips := fs.FileTable.search(remote)
	//fmt.Println(target_ips)
	for _, ip := range target_ips {
		client, err := rpc.Dial("tcp", ip+":"+fs.config.Port)
		if err != nil {
			log.Println(err)
			continue
		}
		var success bool
		err = client.Call("FileRPCServer.LocalAppend", map[string]string{
			"filename": remote,
			"content":  content,
		}, &success)
		if err != nil {
			log.Println(err)
			continue
		}
	}
	var success bool
	err := fs.FileTable.PutEntry(remote, &success)
	if err != nil {
		log.Println(err)
	}
	for id, _ := range fs.MemberInfo.Members {
		client, err := rpc.Dial("tcp", strings.Split(id, "_")[0]+":"+fs.config.Port)
		if err != nil {
			log.Println(err)
			continue
		}
		err = client.Call("FileRPCServer.PutEntry", remote, &success)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}

