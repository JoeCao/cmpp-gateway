# 单元测试指南

本文档介绍如何使用 Mock 编写单元测试，以及单元测试的最佳实践。

## 目录

1. [为什么使用 Mock](#为什么使用-mock)
2. [Mock 的基本使用](#mock-的基本使用)
3. [测试场景示例](#测试场景示例)
4. [最佳实践](#最佳实践)
5. [运行测试](#运行测试)

## 为什么使用 Mock

在单元测试中，我们不应该依赖外部服务（如真实的 CMPP 服务器、数据库等），因为：

1. **速度**：Mock 比真实网络调用快得多
2. **可控性**：可以模拟各种成功/失败场景
3. **隔离性**：测试只关注被测试代码的逻辑，不受外部环境影响
4. **稳定性**：不依赖外部服务的可用性

## Mock 的基本使用

### 1. 定义 Mock 结构体

```go
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
// ... 其他方法类似
```

### 2. 在测试中注入 Mock

```go
func TestSomething(t *testing.T) {
    cm := NewClientManager(config)
    
    // 注入 mock 客户端
    cm.newClient = func() cmppClient {
        return &mockClient{
            connectFunc: func(addr, user, password string, timeout time.Duration) error {
                // 模拟连接成功
                return nil
            },
        }
    }
    
    // 执行测试
    err := cm.Connect()
    // 验证结果
}
```

## 测试场景示例

### 示例 1：测试连接成功

```go
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
```

### 示例 2：测试连接失败

```go
func TestClientManagerConnectFailure(t *testing.T) {
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
}
```

### 示例 3：测试发送请求

```go
func TestClientManagerSendReqPktSuccess(t *testing.T) {
    cm := NewClientManager(config)

    // 使用 mock：模拟发送成功
    seqId := uint32(12345)
    cm.newClient = func() cmppClient {
        return &mockClient{
            connectFunc: func(addr, user, password string, timeout time.Duration) error {
                return nil
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
}
```

### 示例 4：测试并发场景

```go
func TestClientManagerConcurrentConnect(t *testing.T) {
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

    // 并发调用 Connect 100 次
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            _ = cm.Connect() // 忽略错误，只测试并发安全
        }()
    }
    wg.Wait()

    // 最重要的是没有 panic
    cm.Shutdown()
}
```

## 最佳实践

### 1. 测试命名规范

- 测试函数名以 `Test` 开头
- 使用描述性的名称，如 `TestClientManagerConnectSuccess`
- 对于表格驱动测试，使用 `TestXXX_TableDriven`

### 2. 测试结构（AAA 模式）

```go
func TestSomething(t *testing.T) {
    // Arrange（准备）：设置测试数据和 mock
    config := &Config{...}
    cm := NewClientManager(config)
    cm.newClient = func() cmppClient { ... }

    // Act（执行）：执行被测试的操作
    err := cm.Connect()

    // Assert（断言）：验证结果
    if err != nil {
        t.Fatalf("Expected success, got: %v", err)
    }
}
```

### 3. 测试覆盖

- **正常路径**：测试成功场景
- **错误路径**：测试失败场景
- **边界条件**：测试边界值
- **并发场景**：测试并发安全性

### 4. Mock 设计原则

- **只 Mock 外部依赖**：不要 Mock 被测试的代码本身
- **保持简单**：Mock 应该尽可能简单
- **验证调用**：验证 Mock 方法是否被正确调用
- **验证参数**：验证传递给 Mock 的参数是否正确

### 5. 清理资源

```go
func TestSomething(t *testing.T) {
    cm := NewClientManager(config)
    defer cm.Shutdown() // 确保清理资源
    
    // ... 测试代码
}
```

### 6. 测试私有方法

如果方法名以小写字母开头（私有方法），无法直接测试。可以通过以下方式：

1. **通过公共方法间接测试**：
   ```go
   // 无法直接测试 handleActiveTestReq
   // 但可以通过接收消息来间接测试
   cm.StartReceiver()
   // mock 返回心跳请求
   // 验证 handleActiveTestReq 的逻辑
   ```

2. **将方法移到测试包中**：使用 `_test.go` 文件测试内部方法（需要导出）

3. **重构代码**：将复杂逻辑提取到可测试的独立函数

## 运行测试

### 运行所有测试

```bash
go test ./...
```

### 运行特定包的测试

```bash
go test ./gateway
```

### 运行特定测试

```bash
go test ./gateway -run TestClientManagerConnectSuccess
```

### 运行测试并显示详细信息

```bash
go test ./gateway -v
```

### 运行测试并显示覆盖率

```bash
go test ./gateway -cover
```

### 生成覆盖率报告

```bash
go test ./gateway -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 运行基准测试

```bash
go test ./gateway -bench=.
```

### 运行并发测试（检测 race condition）

```bash
go test ./gateway -race
```

## 常见问题

### Q: Mock 什么时候应该返回错误？

A: 当你需要测试错误处理逻辑时。例如：
- 测试连接失败时的行为
- 测试发送失败时的重试逻辑
- 测试网络超时时的处理

### Q: 如何测试异步操作？

A: 使用 `time.Sleep` 或 `sync.WaitGroup` 等待异步操作完成：
```go
cm.StartReceiver()
time.Sleep(200 * time.Millisecond) // 等待协程处理
// 验证结果
```

### Q: 如何测试并发安全性？

A: 使用 `sync.WaitGroup` 和多个 goroutine：
```go
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        // 并发操作
    }()
}
wg.Wait()
```

### Q: 测试失败时如何调试？

A: 
1. 使用 `t.Logf` 输出调试信息
2. 使用 `-v` 标志查看详细输出
3. 使用 `t.Fatalf` 在关键错误时立即停止
4. 检查 Mock 是否被正确调用

## 参考示例

完整的测试示例请参考 `gateway/client_manager_test.go`。

