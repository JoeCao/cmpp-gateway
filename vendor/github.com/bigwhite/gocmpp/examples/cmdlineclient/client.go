package main

import (
	"bufio"
	"fmt"
	"github.com/bigwhite/gocmpp"
	"github.com/bigwhite/gocmpp/utils"
	"log"
	"os"
	//"sync"
	"time"
	"github.com/streamrail/concurrent-map"
	"strconv"
)

const (
	user string = "900001"
	password string = "888888"
	connectTimeout time.Duration = time.Second * 2
)

//var wg sync.WaitGroup
var messages = make(chan string, 10)
var cancel = make(chan struct{})
var submit_cache = cmap.New()

func startAClient(idx int) {
	c := cmpp.NewClient(cmpp.V30)
	//defer wg.Done()
	defer c.Disconnect()
	err := c.Connect(":8888", user, password, connectTimeout)
	if err != nil {
		log.Printf("client %d: connect error: %s.", idx, err)
		return
	}
	log.Printf("client %d: connect and auth ok", idx)
	go func() {
		for {
			if !isRunning() {
				break
			}
			// recv packets
			i, err := c.RecvAndUnpackPkt(0)
			//fmt.Println("2" + context)
			if err != nil {
				log.Printf("client %d: client read and unpack pkt error: %s.", idx, err)
				break
			}

			switch p := i.(type) {
			case *cmpp.Cmpp3SubmitRspPkt:
				log.Printf("client %d: receive a cmpp3 submit response: %v.", idx, p)
				if content, ok := submit_cache.Get(strconv.FormatUint(uint64(p.SeqId), 10)); ok{
					log.Printf("短信内容: %s, 发送状态 %d", content, p.Result)
				}
			case *cmpp.CmppActiveTestReqPkt:
				//log.Printf("client %d: receive a cmpp active request: %v.", idx, p)
				rsp := &cmpp.CmppActiveTestRspPkt{}
				err := c.SendRspPkt(rsp, p.SeqId)
				if err != nil {
					log.Printf("client %d: send cmpp active response error: %s.", idx, err)
					break
				}
			case *cmpp.CmppActiveTestRspPkt:
			//log.Printf("client %d: receive a cmpp activetest response: %v.", idx, p)

			case *cmpp.CmppTerminateReqPkt:
				log.Printf("client %d: receive a cmpp terminate request: %v.", idx, p)
				rsp := &cmpp.CmppTerminateRspPkt{}
				err := c.SendRspPkt(rsp, p.SeqId)
				if err != nil {
					log.Printf("client %d: send cmpp terminate response error: %s.", idx, err)
					break
				}
			case *cmpp.CmppTerminateRspPkt:
				log.Printf("client %d: receive a cmpp terminate response: %v.", idx, p)
			}
		}
	}()
	// recv packets
	//var context string
	for {

		select {
		case context := <-messages:
		//submit a message
			cont, err := cmpputils.Utf8ToUcs2(context)
			if err != nil {
				fmt.Printf("client %d: utf8 to ucs2 transform err: %s.", idx, err)
				return
			}
			p := &cmpp.Cmpp3SubmitReqPkt{
				PkTotal:            1,
				PkNumber:           1,
				RegisteredDelivery: 0,
				MsgLevel:           1,
				ServiceId:          "test",
				FeeUserType:        2,
				FeeTerminalId:      "13500002696",
				FeeTerminalType:    0,
				MsgFmt:             8,
				MsgSrc:             "900001",
				FeeType:            "02",
				FeeCode:            "10",
				ValidTime:          "151105131555101+",
				AtTime:             "",
				SrcId:              "900001",
				DestUsrTl:          1,
				DestTerminalId:     []string{"13500002696"},
				DestTerminalType:   0,
				MsgLength:          uint8(len(cont)),
				MsgContent:         cont,
			}

			seq_id, err := c.SendReqPktWithSeqId(p)
			submit_cache.Set(strconv.FormatUint(uint64(seq_id), 10), context)
			if err != nil {
				log.Printf("client %d: send a cmpp3 submit request error: %s.", idx, err)
			} else {
				log.Printf("client %d: send a cmpp3 submit request ok", idx)
			}
		case <-cancel:
			break
		}
	}


}

func isRunning() bool{
	select {
	case <-cancel:
		return false
	default:
		return true
	}
}

func main() {
	log.Println("Please input sms context, press return to send  and input 'stop' to quit")
	//running := true
	reader := bufio.NewReader(os.Stdin)
	for i := 1; i < 2; i++ {
		go startAClient(i)
	}
	for isRunning() {
		data, _, _ := reader.ReadLine()
		command := string(data)
		messages <- command
		if command == "stop" {
			//running = false
			close(cancel)
		}
		log.Println("command", command)
	}
	<-cancel
}
