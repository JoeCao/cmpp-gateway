package gateway

import (
	"bufio"
	"fmt"
	"github.com/bigwhite/gocmpp"
	"github.com/bigwhite/gocmpp/utils"
	"log"
	"os"
	//"sync"
	"github.com/streamrail/concurrent-map"
	"strconv"
	"time"
)

const (
	user           string        = "104221"
	password       string        = "051992"
	connectTimeout time.Duration = time.Second * 2
)

//var wg sync.WaitGroup
type SmsMessage struct {
	Src string
	Dest string
	Content string
	MsgId string
	Created time.Time
}
var Messages = make(chan SmsMessage, 10)
var cancel = make(chan struct{})
var waitSeqIdCache = cmap.New()
var SubmitCache = cmap.New()

func startAClient(idx int) {
	c := cmpp.NewClient(cmpp.V30)
	ticker := time.NewTicker(time.Second * 30)
	//defer wg.Done()
	defer c.Disconnect()
	err := c.Connect(":7891", user, password, connectTimeout)
	if err != nil {
		log.Printf("client %d: connect error: %s.", idx, err)
		return
	}
	log.Printf("client %d: connect and auth ok", idx)
	go func() {
		for {
			select {
			case <-ticker.C:
				req := &cmpp.CmppActiveTestReqPkt{}
				log.Printf("send test active rep to cmpp server %v", req)
				err := c.SendReqPkt(req)
				if err != nil {
					log.Printf("client %d: send cmpp active response error: %s.", idx, err)
				}
			case <-cancel:
				break
			}
		}
	}()
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
				seqId := strconv.FormatUint(uint64(p.SeqId), 10)
				if mes, ok := waitSeqIdCache.Get(seqId); ok {
					log.Printf("短信内容: %v, 发送状态 %d", mes, p.Result)
					waitSeqIdCache.Remove(seqId)
					SubmitCache.Set(strconv.FormatUint(p.MsgId, 10), mes)
				}
			case *cmpp.CmppActiveTestReqPkt:
				log.Printf("client %d: receive a cmpp active request: %v.", idx, p)
				rsp := &cmpp.CmppActiveTestRspPkt{}
				err := c.SendRspPkt(rsp, p.SeqId)
				//log.Printf("send rsp to cmpp server %v", rsp)
				if err != nil {
					log.Printf("client %d: send cmpp active response error: %s.", idx, err)
					break
				}
			case *cmpp.CmppActiveTestRspPkt:
				log.Printf("client %d: receive a cmpp activetest response: %v.", idx, p)

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
			case *cmpp.Cmpp3DeliverReqPkt:
				log.Printf("client %d: receive a delivery report request: %v", idx, p)
				rsp := &cmpp.Cmpp3DeliverRspPkt{}
				rsp.MsgId = p.MsgId
				rsp.Result = 0
				err := c.SendRspPkt(rsp, p.SeqId)
				if err != nil {
					log.Printf("client %d: send cmpp delivery report request error: %s.", idx, err)
				}

			}
		}
	}()
	// recv packets
	//var context string
	for {

		select {
		case message := <-Messages:
			log.Printf("mes %v", message)
			//submit a message
			cont, err := cmpputils.Utf8ToUcs2(message.Content)
			if err != nil {
				fmt.Printf("client %d: utf8 to ucs2 transform err: %s.", idx, err)
				return
			}
			p := &cmpp.Cmpp3SubmitReqPkt{
				PkTotal:            1,
				PkNumber:           1,
				RegisteredDelivery: 1,
				MsgLevel:           1,
				ServiceId:          "JSASXW",
				FeeUserType:        2,
				FeeTerminalId:      "1064899104221",
				FeeTerminalType:    0,
				MsgFmt:             8,
				MsgSrc:             message.Src,
				FeeType:            "02",
				FeeCode:            "10",
				ValidTime:          "151105131555101+",
				AtTime:             "",
				SrcId:              message.Src,
				DestUsrTl:          1,
				DestTerminalId:     []string{message.Dest},
				DestTerminalType:   0,
				MsgLength:          uint8(len(cont)),
				MsgContent:         cont,
			}

			seq_id, err := c.SendReqPktWithSeqId(p)
			message.Created = time.Now()
			waitSeqIdCache.Set(strconv.FormatUint(uint64(seq_id), 10), message)
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

func isRunning() bool {
	select {
	case <-cancel:
		return false
	default:
		return true
	}
}

func StartInput() {
	log.Println("Please input sms context, press return to send  and input 'stop' to quit")
	//running := true
	reader := bufio.NewReader(os.Stdin)
	for i := 1; i < 2; i++ {
		go startAClient(i)
	}
	for isRunning() {
		data, _, _ := reader.ReadLine()
		command := string(data)
		mes := SmsMessage{Content:command, Src:"104221", Dest:"13900001111"}

		Messages <- mes
		if command == "stop" {
			//running = false
			close(cancel)
		}
		log.Println("command", command)
	}
	<-cancel
}
