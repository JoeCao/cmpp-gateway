package gateway

import (
	"github.com/bigwhite/gocmpp"
	"log"
	"strconv"
	"time"
)

const (
	connectTimeout time.Duration = time.Second * 2
)

//发送消息队列
var Messages = make(chan SmsMes, 10)

//退出消息队列
var Abort = make(chan struct{})

//配置文件
var config *Config

//全局的短信cmpp链接
var c *cmpp.Client

func connectServer() {
	c = cmpp.NewClient(cmpp.V30)
	err := c.Connect(config.CMPPHost+":"+config.CMPPPort, config.User, config.Password, connectTimeout)
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
			_, err := c.SendReqPkt(req)
			if err != nil {
				log.Printf("send cmpp active response error: %s.", err)
				log.Println("begin to reconnect")
				connectServer()
				go startReceiver()
			}
		case <-Abort:
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
			//seqId := strconv.FormatUint(uint64(p.SeqId), 10)
			//从redis中取出seqId为主键的对应发送消息
			mes, err := SCache.GetWaitCache(p.SeqId)
			if err == nil {
				log.Printf("短信内容: %v, 发送状态 %d", mes, p.Result)
				//根据短信网关的返回值给mes赋值
				mes.MsgId = strconv.FormatUint(p.MsgId, 10)
				mes.SubmitResult = p.Result
				SCache.AddSubmits(&mes)
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

			mes := SmsMes{}
			//根据短信网关的返回值给mes赋值
			mes.MsgId = strconv.FormatUint(p.MsgId, 10)
			mes.Src = p.SrcTerminalId
			mes.Content = p.MsgContent
			mes.Created = time.Now()
			mes.Dest = p.DestId
			SCache.AddMoList(&mes)

		}
	}

}

func startSender() {
OuterLoop:
	for {
		select {
		case message := <-Messages:
			log.Printf("mes %v", message)
			p := &cmpp.Cmpp3SubmitReqPkt{
				PkTotal:            1,
				PkNumber:           1,
				RegisteredDelivery: 0,
				MsgLevel:           1,
				ServiceId:          config.ServiceId,
				FeeUserType:        0,
				FeeTerminalId:      "",
				FeeTerminalType:    0,
				MsgFmt:             0,
				MsgSrc:             message.Src,
				FeeType:            "01",
				FeeCode:            "000000",
				ValidTime:          "",
				AtTime:             "",
				SrcId:              config.SmsAccessNo,
				DestUsrTl:          1,
				DestTerminalId:     []string{message.Dest},
				DestTerminalType:   0,
				MsgLength:          uint8(len(message.Content)),
				MsgContent:         message.Content,
			}

			seq_id, err := c.SendReqPkt(p)
			//赋值default value
			message.Created = time.Now()
			message.DelivleryResult = 65535
			message.SubmitResult = 65535
			SCache.SetWaitCache(seq_id, message)
			if err != nil {
				log.Printf("client : send a cmpp3 submit request error: %s.", err)
			} else {
				log.Printf("client: send a cmpp3 submit request ok")
			}
		case <-Abort:
			break OuterLoop
		}
	}

}

func isRunning() bool {
	select {
	case <-Abort:
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
	<-Abort

}
