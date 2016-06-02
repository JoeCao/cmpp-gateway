package gateway

import (
	"fmt"
	"github.com/bigwhite/gocmpp"
	"github.com/bigwhite/gocmpp/utils"
	"log"
	"github.com/streamrail/concurrent-map"
	"strconv"
	"time"
)

const (
	connectTimeout time.Duration = time.Second * 2
)



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
//全局的短信cmpp链接
var c *cmpp.Client

func connectServer() {
	c = cmpp.NewClient(cmpp.V30)
	err := c.Connect(config.CMPPHost + ":" + config.CMPPPort, config.User, config.Password, connectTimeout)
	if err != nil {
		log.Printf("client connect error: %s.", err)
		return
	}
	log.Printf("client connect and auth ok")
}

func activeTimer() {
	ticker := time.NewTicker(time.Second * 10)
OuterLoop:
	for {
		select {
		case <-ticker.C:
			req := &cmpp.CmppActiveTestReqPkt{}
			log.Printf("send test active rep to cmpp server %v", req)
			err := c.SendReqPkt(req)
			if err != nil {
				log.Printf("send cmpp active response error: %s.", err)
				log.Println("begin to reconnect")
				connectServer()
				go startReceiver()
			}
		case <-abort:
			break OuterLoop
		}
	}
}

func startReceiver() {
	for {
		if !isRunning() {
			break
		}
		// recv packets
		i, err := c.RecvAndUnpackPkt(0)
		if err != nil {
			//connect断开后,Recv的不阻塞会导致cpu上升,需要退出goroutine,等待心跳重建接收
			log.Printf("client : client read and unpack pkt error: %s.", err)
			break
		}

		switch p := i.(type) {
		case *cmpp.Cmpp3SubmitRspPkt:
			log.Printf("client : receive a cmpp3 submit response: %v.", p)
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
			log.Printf("client : receive a cmpp active request: %v.", p)
			rsp := &cmpp.CmppActiveTestRspPkt{}
			err := c.SendRspPkt(rsp, p.SeqId)
			if err != nil {
				log.Printf("client : send cmpp active response error: %s.", err)

			}
		case *cmpp.CmppActiveTestRspPkt:
			log.Printf("client : receive a cmpp activetest response: %v.", p)

		case *cmpp.CmppTerminateReqPkt:
			log.Printf("client : receive a cmpp terminate request: %v.", p)
			rsp := &cmpp.CmppTerminateRspPkt{}
			err := c.SendRspPkt(rsp, p.SeqId)
			if err != nil {
				log.Printf("client : send cmpp terminate response error: %s.", err)
				break
			}
		case *cmpp.CmppTerminateRspPkt:
			log.Printf("client : receive a cmpp terminate response: %v.", p)
		case *cmpp.Cmpp3DeliverReqPkt:
			log.Printf("client : receive a delivery report request: %v", p)
			rsp := &cmpp.Cmpp3DeliverRspPkt{}
			rsp.MsgId = p.MsgId
			rsp.Result = 0
			err := c.SendRspPkt(rsp, p.SeqId)
			if err != nil {
				log.Printf("client: send cmpp delivery report request error: %s.", err)
			}

		}
	}

}

func startSender() {
OuterLoop:
	for {
		select {
		case message := <-Messages:
			log.Printf("mes %v", message)
			cont, err := cmpputils.Utf8ToUcs2(message.Content)
			if err != nil {
				fmt.Printf("client : utf8 to ucs2 transform err: %s.", err)
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
			//赋值default value
			message.Created = time.Now()
			message.DelivleryResult = 65535
		        message.SubmitResult = 65535
			waitSeqIdCache.Set(strconv.FormatUint(uint64(seq_id), 10), message)
			if err != nil {
				log.Printf("client : send a cmpp3 submit request error: %s.", err)
			} else {
				log.Printf("client: send a cmpp3 submit request ok")
			}
		case <-abort:
			break OuterLoop
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

func StartClient(gconfig *Config) {
	config = gconfig
	connectServer()
	go startSender()
	go startReceiver()
	go activeTimer()
	defer c.Disconnect()
	<-abort

}
