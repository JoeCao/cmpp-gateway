package gateway

import (
	"log"
	"strconv"
	"time"

	cmpp "github.com/bigwhite/gocmpp"
)

const (
	connectTimeout time.Duration = time.Second * 2
)

// 发送消息队列
var Messages = make(chan SmsMes, 10)

// 退出消息队列
var Abort = make(chan struct{})

// 配置文件
var config *Config

// 全局的短信cmpp链接
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
		// 检查连接是否有效
		if c == nil {
			log.Printf("client连接为空，退出receiver")
			time.Sleep(time.Second) // 避免CPU空转
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
			log.Printf("收到待发送消息: Src=%s, Dest=%s, Content=%s", message.Src, message.Dest, message.Content)

			// 构建实际的发送号码
			// 如果用户提供了扩展码（src），将其附加到SrcId后面
			srcId := config.SmsAccessNo
			if message.Src != "" && message.Src != config.SmsAccessNo {
				// 只有当 src 不是完整号码时才追加
				srcId = config.SmsAccessNo + message.Src
				log.Printf("使用扩展码: %s -> %s", message.Src, srcId)
			}

			// 检查 SrcId 长度（CMPP协议要求最大21字节）
			if len(srcId) > 21 {
				log.Printf("错误: SrcId 长度超过21字节: %s (长度: %d)", srcId, len(srcId))
				// 记录失败消息
				message.Created = time.Now()
				message.SubmitResult = 255 // 255表示本地错误
				message.DelivleryResult = 65535
				message.MsgId = "ERROR"
				SCache.AddSubmits(&message)
				continue
			}

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
				MsgSrc:             config.User, // MsgSrc应该是企业代码，即登录用户名（6字节）
				FeeType:            "01",
				FeeCode:            "000000",
				ValidTime:          "",
				AtTime:             "",
				SrcId:              srcId,
				DestUsrTl:          1,
				DestTerminalId:     []string{message.Dest},
				DestTerminalType:   0,
				MsgLength:          uint8(len(message.Content)),
				MsgContent:         message.Content,
			}

			seq_id, err := c.SendReqPkt(p)
			message.Created = time.Now()
			message.DelivleryResult = 65535

			if err != nil {
				log.Printf("发送CMPP请求失败: %s", err)
				// 发送失败，直接记录到列表，标记为失败
				message.SubmitResult = 254 // 254表示发送失败
				message.MsgId = "SEND_ERROR"
				SCache.AddSubmits(&message)
			} else {
				log.Printf("发送CMPP请求成功，等待响应 (SeqId: %d)", seq_id)
				// 发送成功，等待响应
				message.SubmitResult = 65535 // 等待响应
				SCache.SetWaitCache(seq_id, message)
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
