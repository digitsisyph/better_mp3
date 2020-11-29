package main

import (
	"better_mp3/app/file_service"
	"better_mp3/app/maple_juice_service"
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"
)

var mainChannel = make(chan string)

func Run(s *file_service.FileServer, mj *maple_juice_service.MapleJuiceServer) {
	s.MemberInfo.Run()
	for {
		buf := bufio.NewReader(os.Stdin)
		sentence, err := buf.ReadBytes('\n')
		if err != nil {
			fmt.Println(err)
		} else {
			cmd := strings.Split(string(bytes.Trim([]byte(sentence), "\n")), " ")
			fmt.Println("command: " + cmd[0])
			if cmd[0] == "member" {
				fmt.Println(s.MemberInfo.MemberList.Members)
			} else if cmd[0] == "leave" {
				s.MemberInfo.Leave()
			} else if cmd[0] == "hash" {
				fmt.Println(file_service.MyHash)
			} else if cmd[0] == "ip" {
				fmt.Println(s.MemberInfo.Ip)
			} else if cmd[0] == "id" {
				fmt.Println(s.MemberInfo.Id)
			} else if cmd[0] == "put" {
				if len(cmd) == 3 {
					start := time.Now()
					s.TemptPut(cmd[1], cmd[2])
					fmt.Println(" time to put file is", time.Since(start))
				}
			} else if cmd[0] == "get" {
				if len(cmd) == 3 {
					start := time.Now()
					s.Get(cmd[1], cmd[2])
					fmt.Println(" time to get file is", time.Since(start))
				}
			} else if cmd[0] == "delete" {
				if len(cmd) == 2 {
					start := time.Now()
					s.Delete(cmd[1])
					fmt.Println(" time to delete file is", time.Since(start))
				}
			} else if cmd[0] == "store" {
				s.FileTable.ListMyFiles()
			} else if cmd[0] == "ls" {
				fmt.Println(s.FileTable.ListLocations(cmd[1]))
			} else if cmd[0] == "all" {
				s.FileTable.ListAllFiles()
			} else if cmd[0] == "maple" {
				if len(cmd) == 5 {
					go mj.ScheduleMapleTask(cmd)
				}
			} else if cmd[0] == "juice" {
				if len(cmd) == 5 || len(cmd) == 6 {
					go mj.ScheduleJuiceTask(cmd)
				}
			}
		}
	}
}

func main() {
	fileServer := file_service.NewFileServer()
	go file_service.RunRPCServer(&fileServer)
	mjServer := maple_juice_service.NewMapleJuiceServer(&fileServer)
	go maple_juice_service.RunMapleJuiceRPCServer(&mjServer)
	Run(&fileServer, &mjServer)
	<-mainChannel
}
