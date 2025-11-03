package gateway

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	cmpp "github.com/bigwhite/gocmpp"
)

const (
	// 连接超时时间
	defaultConnectTimeout = 2 * time.Second
	// 心跳间隔
	defaultHeartbeatInterval = 10 * time.Second
)

// ClientManager 管理 CMPP 客户端连接的线程安全封装
type ClientManager struct {
	// 配置
	config *Config

	// CMPP 客户端（需要加锁保护）
	client *cmpp.Client
	mu     sync.RWMutex

	// 连接状态（使用 atomic 保证并发安全）
	ready atomic.Bool

	// 接收协程控制
	receiverRunning atomic.Bool
	receiverStop    chan struct{}
	receiverMu      sync.Mutex // 保护 receiverStop 的创建和关闭

	// 退出信号
	shutdown     chan struct{}
	shutdownOnce sync.Once // 确保只关闭一次
	wg           sync.WaitGroup
}

// NewClientManager 创建一个新的客户端管理器
func NewClientManager(cfg *Config) *ClientManager {
	return &ClientManager{
		config:       cfg,
		shutdown:     make(chan struct{}),
		receiverStop: make(chan struct{}),
	}
}

// Connect 连接到 CMPP 服务器（线程安全）
func (cm *ClientManager) Connect() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 关闭旧连接
	if cm.client != nil {
		cm.client.Disconnect()
		cm.client = nil
	}

	// 创建新连接
	client := cmpp.NewClient(cmpp.V30)
	addr := cm.config.CMPPHost + ":" + cm.config.CMPPPort
	err := client.Connect(addr, cm.config.User, cm.config.Password, defaultConnectTimeout)

	if err != nil {
		Errorf("[CMPP] Connection failed: %v", err)
		cm.ready.Store(false)
		return fmt.Errorf("failed to connect to CMPP server: %w", err)
	}

	cm.client = client
	cm.ready.Store(true)
	Infof("[CMPP] Connection and authentication successful")
	return nil
}

// Disconnect 断开连接（线程安全）
func (cm *ClientManager) Disconnect() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.client != nil {
		cm.client.Disconnect()
		cm.client = nil
		cm.ready.Store(false)
	}
}

// GetClient 获取当前客户端（线程安全的读取）
// 返回的客户端可能为 nil，调用者需要检查
func (cm *ClientManager) GetClient() *cmpp.Client {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.client
}

// IsReady 检查连接是否就绪
func (cm *ClientManager) IsReady() bool {
	return cm.ready.Load()
}

// SendReqPkt 发送请求包（线程安全）
// p 必须实现 cmpp.Packer 接口
func (cm *ClientManager) SendReqPkt(p cmpp.Packer) (uint32, error) {
	cm.mu.RLock()
	client := cm.client
	ready := cm.ready.Load()
	cm.mu.RUnlock()

	if !ready || client == nil {
		return 0, fmt.Errorf("CMPP client not ready")
	}

	return client.SendReqPkt(p)
}

// SendRspPkt 发送响应包（线程安全）
// p 必须实现 cmpp.Packer 接口
func (cm *ClientManager) SendRspPkt(p cmpp.Packer, seqId uint32) error {
	cm.mu.RLock()
	client := cm.client
	ready := cm.ready.Load()
	cm.mu.RUnlock()

	if !ready || client == nil {
		return fmt.Errorf("CMPP client not ready")
	}

	return client.SendRspPkt(p, seqId)
}

// RecvAndUnpackPkt 接收并解包消息（线程安全）
func (cm *ClientManager) RecvAndUnpackPkt(timeout time.Duration) (interface{}, error) {
	cm.mu.RLock()
	client := cm.client
	cm.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("CMPP client not available")
	}

	return client.RecvAndUnpackPkt(timeout)
}

// StartReceiver 启动接收协程（防止重复启动）
func (cm *ClientManager) StartReceiver() {
	// 使用 CompareAndSwap 确保只有一个接收协程在运行
	if !cm.receiverRunning.CompareAndSwap(false, true) {
		Debugf("[CMPP][RECV] Receiver already running, skipping")
		return
	}

	// 重置停止信号
	cm.receiverMu.Lock()
	cm.receiverStop = make(chan struct{})
	stopChan := cm.receiverStop
	cm.receiverMu.Unlock()

	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		defer cm.receiverRunning.Store(false)

		Infof("[CMPP][RECV] Receiver goroutine started")
		cm.receiveLoop(stopChan)
		Infof("[CMPP][RECV] Receiver goroutine stopped")
	}()
}

// StopReceiver 停止接收协程
func (cm *ClientManager) StopReceiver() {
	if cm.receiverRunning.Load() {
		cm.receiverMu.Lock()
		if cm.receiverStop != nil {
			close(cm.receiverStop)
		}
		cm.receiverMu.Unlock()
	}
}

// receiveLoop 接收消息的主循环
func (cm *ClientManager) receiveLoop(stopChan chan struct{}) {
	for {
		select {
		case <-stopChan:
			Debugf("[CMPP][RECV] Received stop signal")
			return
		case <-cm.shutdown:
			Debugf("[CMPP][RECV] Received shutdown signal")
			return
		default:
			// 检查连接状态
			if !cm.IsReady() {
				Debugf("[CMPP][RECV] Client not ready, exiting receiver")
				time.Sleep(time.Second)
				return
			}

			// 接收并处理消息
			pkt, err := cm.RecvAndUnpackPkt(0)
			if err != nil {
				Warnf("[CMPP][RECV] Receive/unpack failed: %v, marking not ready", err)
				cm.ready.Store(false)
				return
			}

			// 处理消息
			cm.handlePacket(pkt)
		}
	}
}

// handlePacket 处理接收到的消息包
func (cm *ClientManager) handlePacket(pkt interface{}) {
	switch p := pkt.(type) {
	case *cmpp.Cmpp3SubmitRspPkt:
		cm.handleSubmitRsp(p)
	case *cmpp.CmppActiveTestReqPkt:
		cm.handleActiveTestReq(p)
	case *cmpp.CmppActiveTestRspPkt:
		cm.handleActiveTestRsp(p)
	case *cmpp.CmppTerminateReqPkt:
		cm.handleTerminateReq(p)
	case *cmpp.CmppTerminateRspPkt:
		cm.handleTerminateRsp(p)
	case *cmpp.Cmpp3DeliverReqPkt:
		cm.handleDeliverReq(p)
	default:
		Debugf("[CMPP][RECV] Unknown packet type: %T", pkt)
	}
}

// handleSubmitRsp 处理提交响应
func (cm *ClientManager) handleSubmitRsp(p *cmpp.Cmpp3SubmitRspPkt) {
	Infof("[CMPP][SUBMIT-RSP] Received submit response: MsgId=%d SeqId=%d Result=%d", p.MsgId, p.SeqId, p.Result)

	// 从缓存中获取等待响应的消息
	mes, err := SCache.GetWaitCache(p.SeqId)
	if err == nil {
		Debugf("[CMPP][SUBMIT-RSP] Matched pending message: %+v, Result=%d", mes, p.Result)
		// 更新消息状态
		mes.MsgId = fmt.Sprintf("%d", p.MsgId)
		mes.SubmitResult = p.Result
		SCache.AddSubmits(&mes)
	} else {
		Warnf("[CMPP][SUBMIT-RSP] No pending message found for SeqId=%d: %v", p.SeqId, err)
	}
}

// handleActiveTestReq 处理心跳请求
func (cm *ClientManager) handleActiveTestReq(p *cmpp.CmppActiveTestReqPkt) {
	Debugf("[CMPP][HEARTBEAT] Received active test request: %+v", p)
	rsp := &cmpp.CmppActiveTestRspPkt{}
	err := cm.SendRspPkt(rsp, p.SeqId)
	if err != nil {
		Errorf("[CMPP][HEARTBEAT] Failed to send active test response: %v", err)
		cm.ready.Store(false)
	}
}

// handleActiveTestRsp 处理心跳响应
func (cm *ClientManager) handleActiveTestRsp(p *cmpp.CmppActiveTestRspPkt) {
	Debugf("[CMPP][HEARTBEAT] Received active test response: %+v", p)
	cm.ready.Store(true)
}

// handleTerminateReq 处理终止请求
func (cm *ClientManager) handleTerminateReq(p *cmpp.CmppTerminateReqPkt) {
	Warnf("[CMPP] Received terminate request: %+v", p)
	rsp := &cmpp.CmppTerminateRspPkt{}
	err := cm.SendRspPkt(rsp, p.SeqId)
	if err != nil {
		Errorf("[CMPP] Failed to send terminate response: %v", err)
	}
}

// handleTerminateRsp 处理终止响应
func (cm *ClientManager) handleTerminateRsp(p *cmpp.CmppTerminateRspPkt) {
	Infof("[CMPP] Received terminate response: %+v", p)
}

// handleDeliverReq 处理上行消息/状态报告
func (cm *ClientManager) handleDeliverReq(p *cmpp.Cmpp3DeliverReqPkt) {
	Infof("[CMPP][DELIVER] Received MO/delivery report: MsgId=%d SeqId=%d", p.MsgId, p.SeqId)

	// 发送响应
	rsp := &cmpp.Cmpp3DeliverRspPkt{
		MsgId:  p.MsgId,
		Result: 0,
	}
	err := cm.SendRspPkt(rsp, p.SeqId)
	if err != nil {
		Errorf("[CMPP][DELIVER] Failed to send response: %v", err)
	}

	// 保存上行消息
	mes := SmsMes{
		MsgId:   fmt.Sprintf("%d", p.MsgId),
		Src:     p.SrcTerminalId,
		Dest:    p.DestId,
		Content: p.MsgContent,
		Created: time.Now(),
	}
	SCache.AddMoList(&mes)
}

// StartHeartbeat 启动心跳协程
func (cm *ClientManager) StartHeartbeat() {
	cm.wg.Add(1)
	go func() {
		defer cm.wg.Done()
		cm.heartbeatLoop()
	}()
}

// heartbeatLoop 心跳主循环
func (cm *ClientManager) heartbeatLoop() {
	ticker := time.NewTicker(defaultHeartbeatInterval)
	defer ticker.Stop()

	Infof("[CMPP][HEARTBEAT] Heartbeat goroutine started")
	defer Infof("[CMPP][HEARTBEAT] Heartbeat goroutine stopped")

	for {
		select {
		case <-ticker.C:
			cm.performHeartbeat()
		case <-cm.shutdown:
			return
		}
	}
}

// performHeartbeat 执行心跳检测和重连
func (cm *ClientManager) performHeartbeat() {
	// 检查是否需要重连
	if !cm.IsReady() || cm.GetClient() == nil {
		Warnf("[CMPP][HEARTBEAT] Client not ready, attempting reconnection")
		cm.ready.Store(false)
		cm.StopReceiver() // 停止旧的接收协程

		if err := cm.Connect(); err != nil {
			Errorf("[CMPP][HEARTBEAT] Reconnection failed: %v", err)
			return
		}

		// 重连成功，启动接收协程
		cm.StartReceiver()
		return
	}

	// 发送心跳
	req := &cmpp.CmppActiveTestReqPkt{}
	Debugf("[CMPP][HEARTBEAT] Sending active test: %+v", req)
	_, err := cm.SendReqPkt(req)
	if err != nil {
		Errorf("[CMPP][HEARTBEAT] Heartbeat send failed: %v, will reconnect", err)
		cm.ready.Store(false)
		cm.StopReceiver() // 停止接收协程

		// 尝试重连
		if err := cm.Connect(); err != nil {
			Errorf("[CMPP][HEARTBEAT] Reconnection after heartbeat failure failed: %v", err)
			return
		}

		// 重连成功，启动接收协程
		cm.StartReceiver()
	}
}

// Shutdown 关闭客户端管理器（可重复调用）
func (cm *ClientManager) Shutdown() {
	cm.shutdownOnce.Do(func() {
		Infof("[CMPP] Shutting down ClientManager...")

		// 发送关闭信号
		close(cm.shutdown)

		// 停止接收协程
		cm.StopReceiver()

		// 等待所有协程退出
		cm.wg.Wait()

		// 断开连接
		cm.Disconnect()

		Infof("[CMPP] ClientManager shutdown complete")
	})
}
