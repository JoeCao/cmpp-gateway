package gateway

import (
	"time"

	cmpp "github.com/bigwhite/gocmpp"
)

// 发送消息队列
var Messages = make(chan SmsMes, 10)

// 退出消息队列
var Abort = make(chan struct{})

// 配置文件
var config *Config

// 全局的 ClientManager（替代原来的 c *cmpp.Client）
var clientManager *ClientManager

// IsCmppReady 检查 CMPP 连接是否就绪（向后兼容）
func IsCmppReady() bool {
	if clientManager == nil {
		return false
	}
	return clientManager.IsReady()
}

// GetClientManager 获取全局 ClientManager（用于测试和内部使用）
func GetClientManager() *ClientManager {
	return clientManager
}

// startSender 启动发送协程
func startSender() {
OuterLoop:
	for {
		select {
		case message := <-Messages:
			Infof("[SEND] Preparing to send: Src=%s Dest=%s Content=%s", message.Src, message.Dest, message.Content)

			// 构建实际的发送号码
			// 如果用户提供了扩展码（src），将其附加到SrcId后面
			srcId := config.SmsAccessNo
			if message.Src != "" && message.Src != config.SmsAccessNo {
				// 只有当 src 不是完整号码时才追加
				srcId = config.SmsAccessNo + message.Src
				Debugf("[SEND] Using extension code: %s -> %s", message.Src, srcId)
			}

			// 检查 SrcId 长度（CMPP协议要求最大21字节）
			if len(srcId) > 21 {
				Errorf("[SEND] SrcId exceeds 21 bytes: %s (len=%d)", srcId, len(srcId))
				// 记录失败消息
				message.Created = time.Now()
				message.SubmitResult = 255 // 255表示本地错误
				message.DelivleryResult = 65535
				message.MsgId = "ERROR"
				SCache.AddSubmits(&message)
				continue
			}

			// 构建 CMPP 提交请求包
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

			// 使用 ClientManager 发送（线程安全）
			seq_id, err := clientManager.SendReqPkt(p)
			message.Created = time.Now()
			message.DelivleryResult = 65535

			if err != nil {
				Errorf("[SEND] CMPP request send failed: %v", err)
				// 发送失败，直接记录到列表，标记为失败
				message.SubmitResult = 254 // 254表示发送失败
				message.MsgId = "SEND_ERROR"
				SCache.AddSubmits(&message)
			} else {
				Infof("[SEND] Sent successfully, waiting for response SeqId=%d", seq_id)
				// 发送成功，等待响应
				message.SubmitResult = 65535 // 等待响应
				SCache.SetWaitCache(seq_id, message)
			}
		case <-Abort:
			break OuterLoop
		}
	}
}

// isRunning 检查服务是否在运行
func isRunning() bool {
	select {
	case <-Abort:
		return false
	default:
		return true
	}
}

// StartClient 启动 CMPP 客户端（新版本使用 ClientManager）
func StartClient(gconfig *Config) {
	config = gconfig

	// 创建 ClientManager
	clientManager = NewClientManager(config)

	// 初始连接
	if err := clientManager.Connect(); err != nil {
		Errorf("[CMPP] Initial connection failed: %v", err)
		// 不要 Fatal，让心跳协程尝试重连
	}

	// 启动发送协程
	go startSender()

	// 如果初始连接成功，启动接收协程
	if clientManager.IsReady() {
		clientManager.StartReceiver()
	}

	// 启动心跳协程（会自动处理重连）
	clientManager.StartHeartbeat()

	// 等待退出信号
	<-Abort

	// 清理资源
	clientManager.Shutdown()
}
