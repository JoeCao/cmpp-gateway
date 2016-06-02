package gateway

import (
	"log"
	"bufio"
	"os"
)

func StartCmdLine() {
	log.Println("Please input sms context, press return to send  and input 'stop' to quit")
	reader := bufio.NewReader(os.Stdin)
	for isRunning() {
		data, _, _ := reader.ReadLine()
		command := string(data)
		mes := SmsMes{Content: command, Src: "104221", Dest: "13900001111"}

		Messages <- mes
		if command == "stop" {
			close(Abort)
			break
		}
		log.Println("command", command)
	}
}
