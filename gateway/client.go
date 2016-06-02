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
	connectTimeout time.Duration = time.Second * 2
)

type SmsMes struct {
	Src     string
	Dest    string
	Content string
	MsgId   string
	Created time.Time
	SubmitResult uint32
	DelivleryResult uint32

}

//发送消息队列
var Messages = make(chan SmsMes, 10)

//退出消息队列
var abort = make(chan struct{})

//等待submit结果返回的缓存
var waitSeqIdCache = cmap.New()

//等待deliver结果返回
var SubmitCache = cmap.New()

//配置文件
var config *Config

var c *cmpp.Client

func connectServer(idx int) {
	c = cmpp.NewClient(cmpp.V30)
	err := c.Connect(config.CMPPHost + ":" + config.CMPPPort, config.User, config.Password, connectTimeout)
	if err != nil {
		log.Printf("client %d: connect error: %s.", idx, err)
		return
	}
}

func startAClient(idx int) {
	connectServer(idx)
	ticker := time.NewTicker(time.Second * 30)
	//defer wg.Done()
	defer c.Disconnect()

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
					connectServer(idx)
				}
			case <-abort:
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
					sms := mes.(SmsMes)
					sms.MsgId = strconv.FormatUint(p.MsgId, 10)
					sms.SubmitResult = p.Result
					SubmitCache.Set(strconv.FormatUint(p.MsgId, 10), sms)
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
				ServiceId:          config.ServiceId,
				FeeUserType:        0,
				FeeTerminalId:      "",
				FeeTerminalType:    0,
				MsgFmt:             8,
				MsgSrc:             message.Src,
				FeeType:            "01",
				FeeCode:            "000000",
				ValidTime:          "",
				AtTime:             "",
				SrcId:              config.SmsAccessNo,
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
		case <-abort:
			break
		}
	}

}

func isRunning() bool {
	select {
	case <-abort:
		return false
	default:
		return true
	}
}

func StartInput(gconfig *Config) {
	config = gconfig
	log.Println("Please input sms context, press return to send  and input 'stop' to quit")
	//running := true
	reader := bufio.NewReader(os.Stdin)
	for i := 1; i < 2; i++ {
		go startAClient(i)
	}
	for isRunning() {
		data, _, _ := reader.ReadLine()
		command := string(data)
		mes := SmsMes{Content: command, Src: "104221", Dest: "13900001111"}

		Messages <- mes
		if command == "stop" {
			//running = false
			close(abort)
		}
		log.Println("command", command)
	}
	<-abort
}
