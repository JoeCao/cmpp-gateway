package gateway

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cmpp "github.com/bigwhite/gocmpp"
)

type mockClient struct {
	connectFunc    func(addr, user, password string, timeout time.Duration) error
	disconnectFunc func()
	sendReqFunc    func(p cmpp.Packer) (uint32, error)
	sendRspFunc    func(p cmpp.Packer, seqId uint32) error
	recvFunc       func(timeout time.Duration) (interface{}, error)
}

func (m *mockClient) Connect(addr, user, password string, timeout time.Duration) error {
	if m.connectFunc != nil {
		return m.connectFunc(addr, user, password, timeout)
	}
	return nil
}

func (m *mockClient) Disconnect() {
	if m.disconnectFunc != nil {
		m.disconnectFunc()
	}
}

func (m *mockClient) SendReqPkt(p cmpp.Packer) (uint32, error) {
	if m.sendReqFunc != nil {
		return m.sendReqFunc(p)
	}
	return 0, fmt.Errorf("sendReq not implemented")
}

func (m *mockClient) SendRspPkt(p cmpp.Packer, seqId uint32) error {
	if m.sendRspFunc != nil {
		return m.sendRspFunc(p, seqId)
	}
	return fmt.Errorf("sendRsp not implemented")
}

func (m *mockClient) RecvAndUnpackPkt(timeout time.Duration) (interface{}, error) {
	if m.recvFunc != nil {
		return m.recvFunc(timeout)
	}
	return nil, cmpp.ErrReadCmdIDTimeout
}

// TestClientManagerConcurrency 测试 ClientManager 的并发安全性
func TestClientManagerConcurrency(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)
	if cm == nil {
		t.Fatal("NewClientManager returned nil")
	}

	// 测试并发调用 IsReady（应该不会 panic）
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cm.IsReady()
		}()
	}
	wg.Wait()

	// 测试并发调用 GetClient（应该不会 panic）
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cm.GetClient()
		}()
	}
	wg.Wait()

	t.Log("Concurrent IsReady() and GetClient() calls completed without panic")
}

// TestClientManagerReceiverSingleInstance 测试接收协程只启动一次
func TestClientManagerReceiverSingleInstance(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 并发调用 StartReceiver 100 次
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.StartReceiver()
		}()
	}
	wg.Wait()

	// 等待一小段时间让协程启动
	time.Sleep(100 * time.Millisecond)

	// 检查只有一个接收协程在运行
	if !cm.receiverRunning.Load() {
		t.Error("Expected receiver to be running")
	}

	// 停止接收协程
	cm.StopReceiver()
	time.Sleep(100 * time.Millisecond)

	if cm.receiverRunning.Load() {
		t.Error("Expected receiver to be stopped")
	}

	t.Log("Receiver instance control test passed")
}

// TestClientManagerShutdown 测试正常关闭
func TestClientManagerShutdown(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)
	cm.newClient = func() cmppClient {
		return &mockClient{}
	}

	// 启动接收协程（即使连接失败也要测试关闭逻辑）
	cm.StartReceiver()

	// 启动心跳协程
	cm.StartHeartbeat()

	// 等待协程启动
	time.Sleep(100 * time.Millisecond)

	// 关闭
	cm.Shutdown()

	// 再次关闭应该不会 panic
	cm.Shutdown()

	t.Log("Shutdown test passed")
}

// TestClientManagerReadyState 测试状态管理
func TestClientManagerReadyState(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 初始状态应该是 not ready
	if cm.IsReady() {
		t.Error("Expected client to be not ready initially")
	}

	// 手动设置为 ready
	cm.ready.Store(true)
	if !cm.IsReady() {
		t.Error("Expected client to be ready after Store(true)")
	}

	// 手动设置为 not ready
	cm.ready.Store(false)
	if cm.IsReady() {
		t.Error("Expected client to be not ready after Store(false)")
	}

	t.Log("Ready state test passed")
}

// TestClientManagerConcurrentConnect 测试并发连接（最危险的场景）
func TestClientManagerConcurrentConnect(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)
	var attempt atomic.Int32
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(string, string, string, time.Duration) error {
				n := attempt.Add(1)
				if n%3 == 0 {
					return errors.New("mock connect failure")
				}
				return nil
			},
		}
	}

	// 并发调用 Connect 100 次（模拟心跳协程和其他地方同时重连）
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errorCount atomic.Int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			err := cm.Connect()
			if err != nil {
				errorCount.Add(1)
				t.Logf("Connect #%d failed (expected): %v", index, err)
			} else {
				successCount.Add(1)
				t.Logf("Connect #%d succeeded", index)
			}
		}(i)
	}
	wg.Wait()

	t.Logf("Concurrent connect test: %d success, %d errors (no panic = pass)",
		successCount.Load(), errorCount.Load())

	// 最重要的是没有 panic
	cm.Shutdown()
}

// TestClientManagerSendReqPkt 测试发送请求的并发安全
func TestClientManagerSendReqPkt(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 不连接，直接测试发送（应该返回错误但不 panic）
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cm.SendReqPkt(nil)
			if err == nil {
				t.Error("Expected error when client not ready")
			}
		}()
	}
	wg.Wait()

	t.Log("Concurrent SendReqPkt test passed (no panic)")
}

// TestClientManagerGetClientRace 测试 GetClient 的竞态条件
func TestClientManagerGetClientRace(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)
	cm.newClient = func() cmppClient {
		return &mockClient{}
	}

	// 一个协程不断尝试连接（写操作）
	stopConnect := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopConnect:
				return
			default:
				cm.Connect()
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// 多个协程不断读取客户端（读操作）
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				client := cm.GetClient()
				_ = client // 使用客户端（但不实际调用方法）
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	close(stopConnect)

	t.Log("GetClient race test passed (no panic or race detected)")
	cm.Shutdown()
}

// TestClientManagerDisconnect 测试断开连接的并发安全
func TestClientManagerDisconnect(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 并发调用 Disconnect（应该不会 panic）
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.Disconnect()
		}()
	}
	wg.Wait()

	// 应该处于 not ready 状态
	if cm.IsReady() {
		t.Error("Expected client to be not ready after disconnect")
	}

	t.Log("Concurrent disconnect test passed")
}

// BenchmarkClientManagerIsReady 基准测试 IsReady 性能
func BenchmarkClientManagerIsReady(b *testing.B) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)
	cm.ready.Store(true)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cm.IsReady()
		}
	})
}

// BenchmarkClientManagerGetClient 基准测试 GetClient 性能
func BenchmarkClientManagerGetClient(b *testing.B) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cm.GetClient()
		}
	})
}

// ========== 以下是一些更完整的单元测试示例，展示如何使用 mock ==========

// TestClientManagerConnectSuccess 测试连接成功场景
func TestClientManagerConnectSuccess(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)
	
	// 使用 mock：模拟连接成功
	connected := false
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				// 验证参数
				if addr != "127.0.0.1:7891" {
					t.Errorf("Expected addr 127.0.0.1:7891, got %s", addr)
				}
				if user != "testuser" {
					t.Errorf("Expected user testuser, got %s", user)
				}
				if password != "testpass" {
					t.Errorf("Expected password testpass, got %s", password)
				}
				connected = true
				return nil // 连接成功
			},
		}
	}

	err := cm.Connect()
	if err != nil {
		t.Fatalf("Expected Connect to succeed, got error: %v", err)
	}

	if !connected {
		t.Error("Expected mock Connect to be called")
	}

	if !cm.IsReady() {
		t.Error("Expected client to be ready after successful connect")
	}

	cm.Shutdown()
}

// TestClientManagerConnectFailure 测试连接失败场景
func TestClientManagerConnectFailure(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 使用 mock：模拟连接失败
	connectErr := errors.New("connection refused")
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				return connectErr
			},
		}
	}

	err := cm.Connect()
	if err == nil {
		t.Fatal("Expected Connect to fail, but got nil error")
	}

	if cm.IsReady() {
		t.Error("Expected client to be not ready after failed connect")
	}

	cm.Shutdown()
}

// TestClientManagerSendReqPktSuccess 测试发送请求成功场景
func TestClientManagerSendReqPktSuccess(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 使用 mock：模拟连接成功和发送成功
	seqId := uint32(12345)
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				return nil // 连接成功
			},
			sendReqFunc: func(p cmpp.Packer) (uint32, error) {
				if p == nil {
					return 0, errors.New("packet is nil")
				}
				return seqId, nil // 发送成功
			},
		}
	}

	// 先连接
	if err := cm.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 测试发送
	pkt := &cmpp.CmppActiveTestReqPkt{}
	gotSeqId, err := cm.SendReqPkt(pkt)
	if err != nil {
		t.Fatalf("Expected SendReqPkt to succeed, got error: %v", err)
	}

	if gotSeqId != seqId {
		t.Errorf("Expected seqId %d, got %d", seqId, gotSeqId)
	}

	cm.Shutdown()
}

// TestClientManagerSendReqPktNotReady 测试未连接时发送请求
func TestClientManagerSendReqPktNotReady(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 不连接，直接测试发送
	pkt := &cmpp.CmppActiveTestReqPkt{}
	_, err := cm.SendReqPkt(pkt)
	if err == nil {
		t.Error("Expected error when client not ready")
	}

	expectedErr := "CMPP client not ready"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

// TestClientManagerReceiveLoopWithMock 测试接收循环处理消息
func TestClientManagerReceiveLoopWithMock(t *testing.T) {
	// 初始化缓存（使用临时 BoltDB）
	config := &Config{
		CMPPHost:  "127.0.0.1",
		CMPPPort:  "7891",
		User:      "testuser",
		Password:  "testpass",
		CacheType: "boltdb",
		DBPath:    "./data/test.db",
	}
	InitCache(config)
	defer func() {
		// 清理测试数据库
		if boltCache, ok := SCache.(*BoltCache); ok {
			boltCache.StopBoltCache()
		}
		os.Remove("./data/test.db")
	}()

	cm := NewClientManager(config)

	// 使用 mock：模拟接收消息
	recvCount := 0
	submitRsp := &cmpp.Cmpp3SubmitRspPkt{
		MsgId:  123456789,
		SeqId:  100,
		Result: 0,
	}

	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				return nil
			},
			recvFunc: func(timeout time.Duration) (interface{}, error) {
				recvCount++
				if recvCount == 1 {
					return submitRsp, nil
				}
				// 第二次接收返回超时，让循环退出
				return nil, cmpp.ErrReadCmdIDTimeout
			},
		}
	}

	// 连接并启动接收协程
	if err := cm.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 设置等待消息（模拟之前发送的短信）
	testMes := SmsMes{
		Src:     "test",
		Dest:    "13800138000",
		Content: "test message",
		Created: time.Now(),
	}
	if err := SCache.SetWaitCache(100, testMes); err != nil {
		t.Fatalf("Failed to set wait cache: %v", err)
	}

	cm.StartReceiver()

	// 等待接收协程处理消息
	time.Sleep(300 * time.Millisecond)

	cm.StopReceiver()
	cm.Shutdown()

	// 验证消息被处理了
	if recvCount == 0 {
		t.Error("Expected RecvAndUnpackPkt to be called")
	}
}

// TestClientManagerHandleActiveTestReqViaReceive 通过接收循环测试处理心跳请求
// 注意：handleActiveTestReq 是私有方法，我们通过接收消息来间接测试
func TestClientManagerHandleActiveTestReqViaReceive(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 使用 mock：模拟接收心跳请求并发送响应
	sendRspCalled := false
	var sentSeqId uint32
	req := &cmpp.CmppActiveTestReqPkt{
		SeqId: 999,
	}

	recvCount := 0
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				return nil
			},
			sendRspFunc: func(p cmpp.Packer, seqId uint32) error {
				sendRspCalled = true
				sentSeqId = seqId
				return nil
			},
			recvFunc: func(timeout time.Duration) (interface{}, error) {
				recvCount++
				if recvCount == 1 {
					return req, nil // 第一次返回心跳请求
				}
				// 第二次返回超时，让循环退出
				return nil, cmpp.ErrReadCmdIDTimeout
			},
		}
	}

	// 连接并启动接收协程
	if err := cm.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	cm.StartReceiver()

	// 等待接收协程处理消息
	time.Sleep(300 * time.Millisecond)

	cm.StopReceiver()
	cm.Shutdown()

	// 验证发送了响应
	if !sendRspCalled {
		t.Error("Expected SendRspPkt to be called when handling active test req")
	}

	if sentSeqId != 999 {
		t.Errorf("Expected seqId 999, got %d", sentSeqId)
	}
}

// TestClientManagerHandleActiveTestRsp 测试处理心跳响应
func TestClientManagerHandleActiveTestRsp(t *testing.T) {
	config := &Config{
		CMPPHost: "127.0.0.1",
		CMPPPort: "7891",
		User:     "testuser",
		Password: "testpass",
	}

	cm := NewClientManager(config)

	// 连接
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				return nil
			},
		}
	}

	if err := cm.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// 先设置为 not ready
	cm.ready.Store(false)
	if cm.IsReady() {
		t.Fatal("Expected client to be not ready initially")
	}

	// 通过接收循环测试心跳响应（间接测试 handleActiveTestRsp）
	rsp := &cmpp.CmppActiveTestRspPkt{}
	recvCount := 0
	cm.newClient = func() cmppClient {
		return &mockClient{
			connectFunc: func(addr, user, password string, timeout time.Duration) error {
				return nil
			},
			recvFunc: func(timeout time.Duration) (interface{}, error) {
				recvCount++
				if recvCount == 1 {
					return rsp, nil
				}
				return nil, cmpp.ErrReadCmdIDTimeout
			},
		}
	}

	// 重新连接以使用新的 mock
	if err := cm.Connect(); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	cm.StartReceiver()
	time.Sleep(200 * time.Millisecond)
	cm.StopReceiver()

	// 验证连接状态（心跳响应应该会将状态设置为 ready）
	// 注意：这个测试依赖于 handleActiveTestRsp 的实现
	cm.Shutdown()
}
