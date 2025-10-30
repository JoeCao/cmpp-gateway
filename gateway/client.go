package gateway

import (
    "strconv"
    "sync/atomic"
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

// 服务就绪状态（CMPP是否已连接可用）
var cmppReady atomic.Bool

func IsCmppReady() bool {
    return cmppReady.Load()
}

func connectServer() {
	c = cmpp.NewClient(cmpp.V30)
	err := c.Connect(config.CMPPHost+":"+config.CMPPPort, config.User, config.Password, connectTimeout)
    if err != nil {
        Errorf("[CMPP] 连接失败: %v", err)
        cmppReady.Store(false)
		return
	}
    Infof("[CMPP] 连接与鉴权成功")
    cmppReady.Store(true)
}

func activeTimer() {
	ticker := time.NewTicker(time.Second * 10)
OuterLoop:
	for {
		select {
		case <-ticker.C:
            // 未就绪则尝试重连，不发送心跳
            if !IsCmppReady() || c == nil {
                Warnf("[CMPP][HEARTBEAT] 未就绪，尝试重连")
                cmppReady.Store(false)
                connectServer()
                if IsCmppReady() {
                    go startReceiver()
                }
                break
            }
            req := &cmpp.CmppActiveTestReqPkt{}
            Debugf("[CMPP][HEARTBEAT] 发送心跳: %+v", req)
            _, err := c.SendReqPkt(req)
            if err != nil {
                Errorf("[CMPP][HEARTBEAT] 心跳发送失败: %v，开始重连", err)
                cmppReady.Store(false)
                connectServer()
                if IsCmppReady() {
                    go startReceiver()
                }
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
        // 若未就绪则退出，等待心跳定时器重连成功后重启接收协程
        if !IsCmppReady() {
            Debugf("[CMPP][RECV] 未就绪，退出接收协程等待重连")
            time.Sleep(time.Second)
            break
        }
		// 检查连接是否有效
        if c == nil {
            Warnf("[CMPP][RECV] 客户端连接为空，退出接收协程")
			time.Sleep(time.Second) // 避免CPU空转
			break
		}
		// recv packets
        i, err := c.RecvAndUnpackPkt(0)
		if err != nil {
			//connect断开后,Recv的不阻塞会导致cpu上升,需要退出goroutine,等待心跳重建接收
            Warnf("[CMPP][RECV] 读取/解包失败: %v，标记未就绪", err)
            cmppReady.Store(false)
			break
		}

		switch p := i.(type) {
		case *cmpp.Cmpp3SubmitRspPkt:
            Infof("[CMPP][SUBMIT-RSP] 收到提交响应: MsgId=%d SeqId=%d Result=%d", p.MsgId, p.SeqId, p.Result)
			//seqId := strconv.FormatUint(uint64(p.SeqId), 10)
			//从redis中取出seqId为主键的对应发送消息
			mes, err := SCache.GetWaitCache(p.SeqId)
			if err == nil {
                Debugf("[CMPP][SUBMIT-RSP] 匹配待回应消息: %+v, 状态=%d", mes, p.Result)
				//根据短信网关的返回值给mes赋值
				mes.MsgId = strconv.FormatUint(p.MsgId, 10)
				mes.SubmitResult = p.Result
				SCache.AddSubmits(&mes)
			}
		case *cmpp.CmppActiveTestReqPkt:
            Debugf("[CMPP][HEARTBEAT] 收到心跳请求: %+v", p)
			rsp := &cmpp.CmppActiveTestRspPkt{}
			err := c.SendRspPkt(rsp, p.SeqId)
			if err != nil {
                Errorf("[CMPP][HEARTBEAT] 心跳响应发送失败: %v", err)
                cmppReady.Store(false)
			}
		case *cmpp.CmppActiveTestRspPkt:
            Debugf("[CMPP][HEARTBEAT] 收到心跳响应: %+v", p)
            cmppReady.Store(true)

		case *cmpp.CmppTerminateReqPkt:
            Warnf("[CMPP] 收到连接终止请求: %+v", p)
			rsp := &cmpp.CmppTerminateRspPkt{}
			err := c.SendRspPkt(rsp, p.SeqId)
			if err != nil {
                Errorf("[CMPP] 终止响应发送失败: %v", err)
				break
			}
		case *cmpp.CmppTerminateRspPkt:
            Infof("[CMPP] 收到终止响应: %+v", p)
		case *cmpp.Cmpp3DeliverReqPkt:
            Infof("[CMPP][DELIVER] 收到上行/状态报告: MsgId=%d SeqId=%d", p.MsgId, p.SeqId)
			rsp := &cmpp.Cmpp3DeliverRspPkt{}
			rsp.MsgId = p.MsgId
			rsp.Result = 0
			err := c.SendRspPkt(rsp, p.SeqId)
			if err != nil {
                Errorf("[CMPP][DELIVER] 回复失败: %v", err)
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
            Infof("[SEND] 待发送: Src=%s Dest=%s Content=%s", message.Src, message.Dest, message.Content)

			// 构建实际的发送号码
			// 如果用户提供了扩展码（src），将其附加到SrcId后面
			srcId := config.SmsAccessNo
            if message.Src != "" && message.Src != config.SmsAccessNo {
				// 只有当 src 不是完整号码时才追加
				srcId = config.SmsAccessNo + message.Src
                Debugf("[SEND] 使用扩展码: %s -> %s", message.Src, srcId)
			}

			// 检查 SrcId 长度（CMPP协议要求最大21字节）
            if len(srcId) > 21 {
                Errorf("[SEND] SrcId 超过21字节: %s (len=%d)", srcId, len(srcId))
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
                Errorf("[SEND] CMPP 请求发送失败: %v", err)
				// 发送失败，直接记录到列表，标记为失败
				message.SubmitResult = 254 // 254表示发送失败
				message.MsgId = "SEND_ERROR"
				SCache.AddSubmits(&message)
			} else {
                Infof("[SEND] 已发送，等待响应 SeqId=%d", seq_id)
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
    if IsCmppReady() {
        go startReceiver()
    }
	go activeTimer()
	defer c.Disconnect()
	<-Abort

}
