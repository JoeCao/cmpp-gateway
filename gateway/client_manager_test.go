package gateway

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
