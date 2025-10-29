# CMPP 3.0 模拟器服务器

这是一个简单的 CMPP 3.0 协议模拟器，用于开发和测试 CMPP 网关，无需连接真实的移动运营商网关。

## 功能特性

- ✅ 支持 CMPP 3.0 和 CMPP 2.0 协议
- ✅ 自动接受所有连接请求（用户名/密码验证通过）
- ✅ 处理短信提交请求（Submit），返回成功响应和唯一 MsgId
- ✅ 支持心跳检测（Active Test）
- ✅ 完整的日志输出，便于调试
- ✅ 自动生成唯一的消息ID（MsgId）

## 快速开始

### 编译

在项目根目录下执行：

```bash
# 编译模拟器
cd simulator
go build -o cmpp-simulator server.go

# 或使用 vendor 模式（推荐）
go build -mod=vendor -o cmpp-simulator server.go
```

### 运行

```bash
# Linux/Mac
./cmpp-simulator

# Windows
cmpp-simulator.exe
```

启动后会看到：

```
==========================================
CMPP 3.0 Simulator Server
==========================================
Listening on: 127.0.0.1:7891
Protocol: CMPP 3.0
Heartbeat interval: 30s
==========================================
Ready to accept connections...
```

## 配置说明

模拟器默认配置：

- **监听地址**: 127.0.0.1:7891
- **协议版本**: CMPP 3.0
- **心跳间隔**: 30秒
- **心跳超时次数**: 3次

如需修改配置，请编辑 `server.go` 中的 `main()` 函数：

```go
addr := "127.0.0.1:7891"  // 修改监听地址和端口
typ := cmpp.V30            // 修改协议版本 (V30 或 V20)
t := 30 * time.Second      // 修改心跳间隔
n := int32(3)              // 修改超时次数
```

## 使用示例

### 1. 启动模拟器

```bash
cd simulator
./cmpp-simulator
```

### 2. 启动 CMPP 网关

修改网关的 `config.json`，确保 CMPP 配置指向模拟器：

```json
{
  "user": "任意用户名",
  "password": "任意密码",
  "cmpp_host": "127.0.0.1",
  "cmpp_port": "7891",
  ...
}
```

然后启动网关：

```bash
cd ..
./cmpp-gateway -c config.json
```

### 3. 发送测试短信

```bash
curl "http://localhost:8000/submit?src=test&dest=13800138000&cont=Hello"
```

在模拟器窗口可以看到：

```
[ConnAuth] CMPP 3.0 connection accepted
[Submit] CMPP 3.0 Submit Request: PkTotal=1, PkNumber=1, DestTerminalId=13800138000, MsgContent=Hello
[Submit] Response: MsgId=123456789, Result=0 (success)
[Heartbeat] Active test request received, responding...
```

## 工作原理

模拟器实现了 4 个主要的 Handler：

### 1. ConnAuthHandler（连接认证）
- 接受所有连接请求
- 返回 Status=0（认证成功）
- 支持 CMPP 2.0 和 3.0

### 2. SubmitHandler（短信提交）
- 接收短信提交请求
- 生成唯一的 MsgId（基于时间戳 + 计数器）
- 返回 Result=0（提交成功）
- 记录短信内容到日志

### 3. ActiveTestHandler（心跳检测）
- 响应客户端的心跳请求
- 主动发送心跳给客户端
- 超过指定次数无响应时断开连接

### 4. TerminateHandler（连接终止）
- 处理客户端主动断开请求
- 清理连接资源

## 日志说明

模拟器会输出详细日志：

| 日志前缀 | 说明 |
|---------|------|
| `[ConnAuth]` | 连接认证相关日志 |
| `[Submit]` | 短信提交相关日志 |
| `[Heartbeat]` | 心跳检测相关日志 |
| `[Terminate]` | 连接终止相关日志 |

## 注意事项

1. **仅供开发测试使用**：此模拟器不验证用户名密码，不限制速率，不保存消息，仅用于开发调试。

2. **所有请求都返回成功**：Submit 请求总是返回 Result=0，不会真正发送短信。

3. **不支持 Deliver 消息**：模拟器不会主动推送上行短信（MO）或状态报告。

4. **端口占用**：确保 7891 端口未被占用，否则启动会失败。

5. **并发连接**：支持多个客户端同时连接，每个连接独立处理。

## 故障排除

### 问题：启动失败 "bind: address already in use"
**解决**：端口 7891 已被占用，修改 `addr` 配置或停止占用端口的程序。

```bash
# Linux/Mac 查看端口占用
lsof -i :7891

# Windows 查看端口占用
netstat -ano | findstr :7891
```

### 问题：网关无法连接到模拟器
**解决**：
1. 检查模拟器是否正常启动
2. 检查 config.json 中的 cmpp_host 和 cmpp_port 配置
3. 检查防火墙是否拦截了端口

### 问题：心跳超时断开连接
**解决**：增大心跳间隔或超时次数：

```go
t := 60 * time.Second  // 改为 60 秒
n := int32(5)          // 改为 5 次
```

## 与真实网关的差异

| 特性 | 模拟器 | 真实网关 |
|-----|--------|---------|
| 用户认证 | 不验证 | 严格验证 |
| 消息发送 | 伪发送 | 真实发送 |
| 状态报告 | 不支持 | 支持 |
| 上行短信 | 不支持 | 支持 |
| 速率限制 | 无限制 | 有限制 |
| 计费 | 不计费 | 计费 |

## 扩展开发

如需添加更多功能，可以修改 Handler：

### 示例：添加用户验证

```go
func (h *ConnAuthHandler) ServeCmpp(r *cmpp.Response, p *cmpp.Packet, l *log.Logger) (bool, error) {
    switch req := p.Packer.(type) {
    case *cmpp.CmppConnReqPkt:
        // 验证用户名和密码
        validUser := "testuser"
        if string(req.SourceAddr) != validUser {
            switch rsp := r.Packer.(type) {
            case *cmpp.Cmpp3ConnRspPkt:
                rsp.Status = 1 // 认证失败
            }
            return false, nil // 断开连接
        }
    }
    return true, nil
}
```

### 示例：模拟 Deliver 消息（上行短信）

```go
// 在连接成功后主动推送一条测试短信
func sendTestDeliver(conn *cmpp.Conn) {
    deliver := &cmpp.Cmpp3DeliverReqPkt{
        MsgId:            12345678,
        DestId:           []byte("10086"),
        ServiceId:        []byte("TEST"),
        SrcTerminalId:    []byte("13800138000"),
        RegisterDelivery: 0,
        MsgContent:       []byte("This is a test MO message"),
    }
    conn.SendPkt(deliver, <-conn.SeqId)
}
```

## 许可证

本模拟器使用与 cmpp-gateway 相同的许可证。基于 [gocmpp](https://github.com/bigwhite/gocmpp) 库开发。
