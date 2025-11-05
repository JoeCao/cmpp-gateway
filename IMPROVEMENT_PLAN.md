# CMPP Gateway 代码质量改进计划

## 优先级说明
- **P0 (Critical)**: 可能导致崩溃或安全问题，必须立即修复
- **P1 (High)**: 严重影响稳定性或性能，应尽快修复
- **P2 (Medium)**: 代码质量问题，影响可维护性
- **P3 (Low)**: 优化建议，可以延后处理

---

## 第一阶段：修复严重问题 (P0)

### 1. ✅ 修复并发安全问题 (P0) - 已完成
**问题位置**: `gateway/client.go:34-44, 53-70`

**问题描述**:
- 全局变量 `c *cmpp.Client` 在多个 goroutine 中无锁写入
- `activeTimer()` 和 `startReceiver()` 同时调用 `connectServer()` 会产生竞态
- 可能导致: panic, 内存泄漏, 连接混乱

**修复提交**: `f006df2` (feat: implement thread-safe ClientManager for CMPP connections)
**修复方式**: 使用 `ClientManager` 封装 + `sync.RWMutex` + `atomic.Bool`

**解决方案**:
```go
// 方案A: 使用互斥锁保护客户端
var (
    c       *cmpp.Client
    cMutex  sync.RWMutex  // 保护 c 的读写
)

func connectServer() {
    cMutex.Lock()
    defer cMutex.Unlock()

    // 关闭旧连接
    if c != nil {
        c.Disconnect()
    }

    c = cmpp.NewClient(cmpp.V30)
    // ... 连接逻辑
}

func getClient() *cmpp.Client {
    cMutex.RLock()
    defer cMutex.RUnlock()
    return c
}
```

**方案B (推荐): 封装为线程安全的客户端管理器**
```go
type ClientManager struct {
    client     *cmpp.Client
    mu         sync.RWMutex
    config     *Config
    ready      atomic.Bool
    reconnChan chan struct{} // 重连信号通道
}

func (cm *ClientManager) Connect() error
func (cm *ClientManager) GetClient() (*cmpp.Client, error)
func (cm *ClientManager) IsReady() bool
```

**影响范围**:
- `gateway/client.go` (主要修改)
- `gateway/httpserver.go` (调用方式调整)
- `main.go` (初始化方式调整)

**预计工作量**: 4-6小时

---

### 2. ✅ 修复 goroutine 泄漏 (P0) - 已完成
**问题位置**: `gateway/client.go:58, 69`

**问题描述**:
- 重连时反复启动 `go startReceiver()` 但旧的可能未退出
- 长时间运行会导致大量僵尸 goroutine

**修复提交**: `b70c9da` (fix: 修复 ClientManager 并发安全问题)
**修复方式**:
- 使用 `atomic.Bool` + `CompareAndSwap` 防止重复启动
- 引入 `receiverDone` channel 确保优雅退出
- 添加 `defaultReceiveTimeout` 避免阻塞
- 精细的错误分类处理（超时、断开、致命错误）

**解决方案**:
```go
type receiverController struct {
    stopChan chan struct{}
    running  atomic.Bool
}

func (rc *receiverController) start() {
    if !rc.running.CompareAndSwap(false, true) {
        return // 已在运行
    }

    rc.stopChan = make(chan struct{})
    go rc.run()
}

func (rc *receiverController) stop() {
    if rc.running.CompareAndSwap(true, false) {
        close(rc.stopChan)
    }
}

func (rc *receiverController) run() {
    defer rc.running.Store(false)

    for {
        select {
        case <-rc.stopChan:
            return
        default:
            // 接收逻辑
        }
    }
}
```

**验证方法**:
```bash
# 运行后检查 goroutine 数量
go tool pprof http://localhost:8000/debug/pprof/goroutine
```

**预计工作量**: 3-4小时

---

### 3. ✅ 修复 HTTP 注入风险 (P0) - 已完成
**问题位置**: `gateway/httpserver.go:20-48`

**问题描述**:
- `src`, `dest`, `cont` 参数未验证直接使用
- 可能注入攻击: XSS (模板注入), 超长内容导致费用损失

**修复提交**: (当前提交)
**修复方式**:
- 新增 `gateway/validation.go` - 参数验证模块
- 新增 `gateway/validation_test.go` - 完整的单元测试（100% 测试覆盖）
- 手机号正则验证: `1[3-9]\d{9}`
- 扩展码正则验证: `\d{1,6}`
- 内容长度限制: 最大 500 字符
- HTML 转义防止 XSS: `html.EscapeString()`
- 分页参数验证: 1-10000 范围限制
- 搜索参数验证: 关键词最大 100 字符

**解决方案**:
```go
import (
    "regexp"
    "unicode/utf8"
)

var (
    // 手机号正则: 1[3-9]\d{9}
    phoneRegex = regexp.MustCompile(`^1[3-9]\d{9}$`)
    // 扩展码: 纯数字，1-6位
    extRegex = regexp.MustCompile(`^\d{1,6}$`)
)

func validateSubmitParams(src, dest, content string) error {
    // 1. 验证目标号码
    if !phoneRegex.MatchString(dest) {
        return fmt.Errorf("无效的手机号: %s", dest)
    }

    // 2. 验证扩展码（可选）
    if src != "" && !extRegex.MatchString(src) {
        return fmt.Errorf("无效的扩展码: %s (仅支持1-6位数字)", src)
    }

    // 3. 验证内容长度（70字符 ≈ 1条短信）
    if utf8.RuneCountInString(content) == 0 {
        return fmt.Errorf("短信内容不能为空")
    }
    if utf8.RuneCountInString(content) > 500 {
        return fmt.Errorf("短信内容过长 (最大500字符)")
    }

    // 4. 过滤危险字符（防止模板注入）
    content = template.HTMLEscapeString(content)

    return nil
}

func handler(w http.ResponseWriter, r *http.Request) {
    // ... 解析表单

    if err := validateSubmitParams(src, dest, content); err != nil {
        w.Header().Set("Content-Type", "application/json; charset=UTF-8")
        result, _ := json.Marshal(
            map[string]interface{}{"result": -1, "error": err.Error()})
        fmt.Fprintf(w, string(result))
        return
    }

    // ... 继续处理
}
```

**预计工作量**: 2-3小时

---

### 4. 敏感信息加密 (P0)
**问题位置**: `gateway/config.go:13, 29`

**问题描述**:
- 密码明文存储在 `config.json`
- Git 历史可能泄漏凭证

**解决方案**:

**方案A: 环境变量覆盖 (快速方案)**
```go
func (c *Config) LoadFile(path string) {
    // ... 读取 JSON

    // 环境变量覆盖敏感配置
    if envPwd := os.Getenv("CMPP_PASSWORD"); envPwd != "" {
        c.Password = envPwd
    }
    if envRedisPwd := os.Getenv("REDIS_PASSWORD"); envRedisPwd != "" {
        c.RedisPassword = envRedisPwd
    }
}
```

**方案B: AES 加密配置 (推荐)**
```go
// config.json 使用加密后的密码
{
    "password": "enc:AES256:a1b2c3d4...",
    "redis_password": "enc:AES256:e5f6g7h8..."
}

// 解密工具
func decryptConfig(encrypted string) (string, error) {
    if !strings.HasPrefix(encrypted, "enc:AES256:") {
        return encrypted, nil // 未加密，直接返回
    }

    // ... AES-256 解密逻辑
}

// 提供 CLI 工具生成加密密码
// go run cmd/encrypt.go "my_password"
```

**配置加密工具**:
```bash
# 新建工具
./cmpp-gateway encrypt "my_password"
# 输出: enc:AES256:a1b2c3d4...

# 将输出写入 config.json
```

**预计工作量**: 4-5小时

---

### 5. 修复 Redis 全表扫描性能问题 (P0)
**问题位置**: `gateway/cache.go:332`

**问题描述**:
- `LRANGE list_message 0 -1` 在数据量大时导致 Redis 阻塞
- 1万条记录 ≈ 1MB 数据传输，可能超时

**解决方案**:

**方案A: 分批扫描 (兼容现有代码)**
```go
func (c *Cache) SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes {
    // 不再全量读取，而是分批扫描直到找到足够的匹配项
    batchSize := 100
    offset := 0
    matchedCount := 0
    result := make([]SmsMes, 0)

    for {
        // 分批读取
        values, err := redis.Strings(conn.Do("LRANGE", listName, offset, offset+batchSize-1))
        if err != nil || len(values) == 0 {
            break
        }

        for _, s := range values {
            mes := SmsMes{}
            if json.Unmarshal([]byte(s), &mes) != nil {
                continue
            }

            if c.matchFilters(&mes, filters, listName) {
                if matchedCount >= start && matchedCount <= end {
                    result = append(result, mes)
                }
                matchedCount++

                // 已收集足够的数据
                if len(result) >= (end - start + 1) {
                    return &result
                }
            }
        }

        offset += batchSize
    }

    return &result
}
```

**方案B: 使用 Redis Sorted Set (架构优化)**
```go
// 修改存储结构
// list_message -> sorted_message (使用 timestamp 作为 score)

func (c *Cache) AddSubmits(mes *SmsMes) error {
    data, _ := json.Marshal(mes)
    score := float64(time.Now().UnixNano())

    // ZADD sorted_message {timestamp} {json}
    _, err := conn.Do("ZADD", "sorted_message", score, data)

    // 支持高效的范围查询和排序
    // ZREVRANGE sorted_message 0 49  (获取最新50条)
    return err
}
```

**注意**: 方案B需要数据迁移，建议先用方案A

**预计工作量**: 3-4小时 (方案A) / 8-10小时 (方案B)

---

## 第一阶段总结

**总工作量**: 约 20-26 小时 (3-4 工作日)

**修复顺序建议**:
1. **Day 1**: 并发安全 (#1) + goroutine 泄漏 (#2)
2. **Day 2**: HTTP 注入 (#3) + 敏感信息加密 (#4)
3. **Day 3**: Redis 性能优化 (#5) + 集成测试

**验证清单**:
- [ ] 使用 `go test -race` 检测竞态条件
- [ ] 使用 `pprof` 确认无 goroutine 泄漏
- [ ] 使用 `ab` 或 `wrk` 进行压力测试
- [ ] 使用 `redis-cli --latency` 检测 Redis 响应时间
- [ ] 手动测试 SQL 注入/XSS 攻击

**回归测试要求**:
```bash
# 1. 功能测试
curl "http://localhost:8000/submit?src=&dest=13800138000&cont=test"

# 2. 并发测试 (100并发，1000请求)
ab -n 1000 -c 100 "http://localhost:8000/submit?dest=13800138000&cont=test"

# 3. 长时间稳定性测试 (运行24小时)
while true; do curl "..." ; sleep 1; done

# 4. 竞态检测
go test -race ./gateway/...
```

---

## 第二阶段：修复设计缺陷 (P1)

### 6. 重构全局状态 (P1)
**当前问题**: 10+ 个包级全局变量，无法测试，不支持多实例

**目标架构**:
```go
// gateway/gateway.go
type Gateway struct {
    config       *Config
    client       *ClientManager
    cache        CacheInterface
    httpServer   *http.Server
    shutdownChan chan struct{}
}

func NewGateway(cfg *Config) (*Gateway, error)
func (g *Gateway) Start() error
func (g *Gateway) Shutdown() error
```

**预计工作量**: 8-10小时

---

### 7. 统一错误处理 (P1)
**当前问题**: `json.Marshal` 错误全部被 `_` 吞噬

**解决方案**:
```go
// 定义错误处理策略
func marshalOrPanic(v interface{}) []byte {
    data, err := json.Marshal(v)
    if err != nil {
        // 这种错误通常是程序bug，应该panic
        panic(fmt.Sprintf("json.Marshal failed: %v", err))
    }
    return data
}

// 或者返回错误
func marshalOrError(v interface{}) ([]byte, error) {
    data, err := json.Marshal(v)
    if err != nil {
        return nil, fmt.Errorf("序列化失败: %w", err)
    }
    return data, nil
}
```

**预计工作量**: 2-3小时

---

### 8. 删除重复的标准库代码 (P1)
**问题**: `gateway/utils.go:14-217` 完全复制 `container/list`

**解决方案**:
```go
// 直接使用标准库
import "container/list"

// 如果确实需要自定义，应该:
// 1. 移除版权声明（因为已大幅修改）
// 2. 添加注释说明为何不用标准库
```

**预计工作量**: 0.5小时 (删除即可)

---

### 9. 消除重复代码 (P1)
**问题**: `boltcache.go:matchFilters` 和 `cache.go:matchFilters` 完全重复

**解决方案**:
```go
// gateway/search.go
package gateway

// MessageFilter 提取公共搜索逻辑
type MessageFilter struct {
    Src     string
    Dest    string
    Content string
    Status  *int // nil表示不过滤
}

func (f *MessageFilter) Match(mes *SmsMes, listType string) bool {
    // 统一的匹配逻辑
}
```

**预计工作量**: 2-3小时

---

## 第三阶段：改进代码质量 (P2)

### 10. 添加单元测试 (P2)
**目标覆盖率**: 60%+

**优先测试模块**:
```bash
gateway/
├── client_test.go          # 测试 ClientManager
├── cache_test.go           # 测试 Redis 实现
├── boltcache_test.go       # 测试 BoltDB 实现
├── httpserver_test.go      # 测试 HTTP handlers
└── search_test.go          # 测试搜索逻辑
```

**预计工作量**: 12-16小时

---

### 11. 统一日志语言 (P2)
**问题**: 中英文混杂

**解决方案**:
```go
// 选择一种语言并统一
// 推荐: 英文（便于国际化）
Infof("[CMPP] Connection and authentication successful")
Errorf("[CMPP] Connection failed: %v", err)

// 或者: 中文（便于国内团队协作）
Infof("[CMPP] 连接与鉴权成功")
Errorf("[CMPP] 连接失败: %v", err)
```

**预计工作量**: 1-2小时

---

### 12. 定义常量替换魔法数字 (P2)
**问题**: `65535`, `254`, `255` 等魔法数字遍布代码

**解决方案**:
```go
// gateway/constants.go
package gateway

const (
    // CMPP 提交结果状态码
    SubmitResultSuccess     uint32 = 0     // 成功
    SubmitResultWaiting     uint32 = 65535 // 等待响应
    SubmitResultLocalError  uint32 = 255   // 本地错误(参数验证失败)
    SubmitResultSendError   uint32 = 254   // 发送失败(网络错误)

    // CMPP 投递结果状态码
    DeliveryResultWaiting   uint32 = 65535 // 等待状态报告

    // HTTP 错误码
    HTTPErrorParams         int = -1  // 参数错误
    HTTPErrorNotReady       int = -2  // 服务未就绪
)
```

**预计工作量**: 2-3小时

---

## 第四阶段：优化建议 (P3)

### 13. 添加性能监控 (P3)
```go
import "github.com/prometheus/client_golang/prometheus"

var (
    smsSubmitTotal = prometheus.NewCounterVec(...)
    smsSubmitDuration = prometheus.NewHistogramVec(...)
)
```

**预计工作量**: 4-6小时

---

### 14. 添加配置热更新 (P3)
```go
func (g *Gateway) ReloadConfig(newCfg *Config) error
```

**预计工作量**: 6-8小时

---

### 15. 添加健康检查接口 (P3)
```go
// GET /health
{
    "status": "healthy",
    "cmpp_connected": true,
    "cache_available": true,
    "uptime_seconds": 12345
}
```

**预计工作量**: 2-3小时

---

## 总工作量估算

| 阶段 | 优先级 | 工作量 | 建议时间 |
|------|--------|--------|----------|
| 第一阶段 | P0 | 20-26h | Week 1 |
| 第二阶段 | P1 | 13-17h | Week 2 |
| 第三阶段 | P2 | 15-21h | Week 3 |
| 第四阶段 | P3 | 12-17h | Week 4 |
| **总计** | - | **60-81h** | **1个月** |

---

## 立即行动

**本周任务 (P0 严重问题)**:
1. ✅ 创建此改进计划文档
2. ⬜ 创建测试分支 `git checkout -b fix/critical-issues`
3. ⬜ 修复并发安全问题 (#1)
4. ⬜ 修复 goroutine 泄漏 (#2)
5. ⬜ 修复 HTTP 注入风险 (#3)
6. ⬜ 实现敏感信息加密 (#4)
7. ⬜ 优化 Redis 搜索性能 (#5)
8. ⬜ 执行回归测试
9. ⬜ Code Review
10. ⬜ 合并到主分支

**开始命令**:
```bash
git checkout -b fix/critical-issues
git add IMPROVEMENT_PLAN.md
git commit -m "docs: add code quality improvement plan"
```
