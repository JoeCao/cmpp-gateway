package gateway

import (
	"bufio"
	"log"
	"os"
)

func StartCmdLine() {
	log.Println("Please input sms context, press return to send  and input 'stop' to quit")
	reader := bufio.NewReader(os.Stdin)
	for isRunning() {
		data, _, _ := reader.ReadLine()
		command := string(data)

		// 只有输入不为空且不是 "stop" 时才发送短信
		if command != "" && command != "stop" {
			mes := SmsMes{Content: command, Src: "104221", Dest: "13900001111"}
			Messages <- mes
			log.Println("发送短信:", command)
		}

		if command == "stop" {
			close(Abort)
			break
		}

		if command == "" {
			log.Println("空输入，跳过发送")
		}
	}
}
