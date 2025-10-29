# BoltDB 迁移指南

## 🎉 重构完成

已成功将 Redis 替换为 BoltDB，实现以下目标：
- ✅ **纯 Go 实现**：无需 CGO，跨平台编译零障碍
- ✅ **零外部依赖**：不需要安装 Redis 服务
- ✅ **单一二进制**：编译后一个文件即可运行
- ✅ **向后兼容**：仍支持 Redis（通过配置切换）

---

## 📦 架构说明

### 实现方案
使用 **BoltDB (bbolt)** 作为嵌入式键值数据库：
- **项目地址**: https://github.com/etcd-io/bbolt
- **成熟度**: etcd、Consul 等知名项目在使用
- **特性**: 
  - 纯 Go 实现，无需 CGO
  - ACID 事务支持
  - 内存映射文件，读操作极快
  - 单文件数据库，易于备份

### 数据映射

| 原 Redis 结构 | BoltDB 实现 | 说明 |
|--------------|-------------|------|
| `HSET waitseqcache` | Bucket: `wait` | 等待响应的消息队列 |
| `LPUSH list_message` | Bucket: `messages` | 已发送消息列表 |
| `LPUSH list_mo` | Bucket: `mo` | 接收的MO消息列表 |

### 接口设计

定义了统一的 `CacheInterface` 接口：
- `SetWaitCache()` - 设置等待消息
- `GetWaitCache()` - 获取并删除等待消息
- `AddSubmits()` - 添加发送消息
- `AddMoList()` - 添加MO消息
- `GetList()` - 获取消息列表
- `GetStats()` - 获取统计信息
- `Length()` - 获取列表长度

两种实现均实现此接口：
- `Cache` - Redis 实现（原有）
- `BoltCache` - BoltDB 实现（新增）

---

## 🚀 使用方法

### 1. 使用 BoltDB（推荐，默认）

配置文件 `config.boltdb.json`：
```json
{
  "user": "104221",
  "password": "051992",
  "sms_accessno": "1064899104221",
  "service_id": "JSASXW",
  "http_host": "0.0.0.0",
  "http_port": "8000",
  "cmpp_host": "127.0.0.1",
  "cmpp_port": "7891",
  "debug": true,
  "cache_type": "boltdb",
  "db_path": "./data/cmpp.db"
}
```

启动：
```bash
# 编译
go build -o cmpp-gateway

# 运行（使用 BoltDB）
./cmpp-gateway -c config.boltdb.json

# 或者直接使用默认配置（会自动使用BoltDB）
./cmpp-gateway
```

**配置说明**：
- `cache_type`: 缓存类型，可选 `boltdb`（默认）或 `redis`
- `db_path`: 数据库文件路径，默认为 `./data/cmpp.db`

### 2. 继续使用 Redis（可选）

配置文件 `config.json`：
```json
{
  "user": "104221",
  "password": "051992",
  "sms_accessno": "1064899104221",
  "service_id": "JSASXW",
  "http_host": "0.0.0.0",
  "http_port": "8000",
  "cmpp_host": "127.0.0.1",
  "cmpp_port": "7891",
  "debug": true,
  "cache_type": "redis",
  "redis_host": "127.0.0.1",
  "redis_port": "6379",
  "redis_password": "123456"
}
```

启动：
```bash
./cmpp-gateway -c config.json
```

---

## 📂 数据文件

使用 BoltDB 后，数据存储在本地文件中：

```
cmpp-gateway/
├── cmpp-gateway          # 主程序
├── config.boltdb.json    # BoltDB配置
├── config.json           # Redis配置（可选）
└── data/                 # 数据目录
    └── cmpp.db          # BoltDB数据文件
```

### 数据备份

BoltDB 使用单文件存储，备份非常简单：

```bash
# 备份
cp ./data/cmpp.db ./data/cmpp.db.backup

# 或使用日期
cp ./data/cmpp.db ./data/cmpp.db.$(date +%Y%m%d)

# 恢复
cp ./data/cmpp.db.backup ./data/cmpp.db
```

### 查看数据库内容

可以使用 `bbolt` 命令行工具查看数据库：

```bash
# 安装 bbolt 工具
go install go.etcd.io/bbolt/cmd/bbolt@latest

# 查看数据库信息
bbolt info ./data/cmpp.db

# 查看 buckets
bbolt buckets ./data/cmpp.db

# 查看指定 bucket 的数据
bbolt get ./data/cmpp.db messages
```

---

## 🔄 从 Redis 迁移到 BoltDB

### 自动迁移（未实现）

目前不支持自动迁移，因为：
1. BoltDB 是全新的数据文件，不会与 Redis 冲突
2. 历史数据通常不需要迁移（消息是短暂的）
3. 重启后即可使用新的缓存系统

### 手动迁移（如需要）

如果确实需要迁移历史数据，可以：

1. 同时连接 Redis 和 BoltDB
2. 从 Redis 读取数据
3. 写入 BoltDB

示例代码（参考）：
```go
// TODO: 如有需要，可以实现迁移脚本
```

---

## ⚙️ 编译说明

### 标准编译

```bash
# 下载依赖
go mod tidy

# 同步 vendor（如果使用 vendor）
go mod vendor

# 编译
go build -o cmpp-gateway

# 或忽略 vendor 编译
go build -mod=mod -o cmpp-gateway
```

### 跨平台编译

BoltDB 是纯 Go 实现，支持跨平台编译：

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o cmpp-gateway-linux

# Windows
GOOS=windows GOARCH=amd64 go build -o cmpp-gateway.exe

# macOS (ARM)
GOOS=darwin GOARCH=arm64 go build -o cmpp-gateway-darwin-arm64

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o cmpp-gateway-darwin-amd64
```

**无需在目标平台安装任何依赖！**

---

## 🔍 性能对比

### BoltDB vs Redis

| 特性 | BoltDB | Redis |
|-----|--------|-------|
| **部署** | 单文件，零配置 | 需要安装服务 |
| **内存** | 按需使用（mmap） | 全部数据在内存 |
| **读性能** | 极快（内存映射） | 极快（纯内存） |
| **写性能** | 较快（事务） | 极快（异步） |
| **持久化** | 自动 | 需配置 |
| **并发** | 单写多读 | 高并发 |
| **适用场景** | 中小规模、单机 | 大规模、分布式 |

### 实际测试

对于 CMPP 网关的使用场景（中小规模短信）：
- ✅ BoltDB 完全满足性能需求
- ✅ 简化部署和运维
- ✅ 降低系统复杂度

---

## 🐛 已知问题

### 1. Vendor 模式编译

如果遇到 vendor 相关错误：
```bash
go: inconsistent vendoring in /path/to/cmpp-gateway
```

解决方案：
```bash
# 重新生成 vendor
go mod vendor

# 或使用 -mod=mod 忽略 vendor
go build -mod=mod -o cmpp-gateway
```

### 2. 文件锁定

BoltDB 使用文件锁，同一时间只能有一个进程打开数据库。

解决方案：
- 确保只运行一个实例
- 或使用不同的数据库文件路径

### 3. 数据库损坏

极少情况下（如进程被 kill -9），可能导致数据库损坏。

预防措施：
- 定期备份数据文件
- 使用优雅关闭（SIGTERM 而非 SIGKILL）

---

## 📊 性能优化建议

### 1. 限制历史记录数量

BoltDB 是文件数据库，数据量过大会影响性能。建议：

```go
// 在添加消息时，限制最大记录数
// TODO: 可以在未来实现自动清理旧数据
```

### 2. 定期压缩数据库

```bash
# 使用 bbolt 工具压缩
bbolt compact -o ./data/cmpp.compact.db ./data/cmpp.db
mv ./data/cmpp.compact.db ./data/cmpp.db
```

### 3. 批量操作

利用 BoltDB 的事务特性，批量写入可以提升性能：

```go
// 目前是单条写入，性能足够
// 如有需要，可以改为批量事务
```

---

## 🎯 推荐配置

### 开发环境

```json
{
  "cache_type": "boltdb",
  "db_path": "./data/cmpp-dev.db",
  "debug": true
}
```

### 生产环境

```json
{
  "cache_type": "boltdb",
  "db_path": "/var/lib/cmpp-gateway/cmpp.db",
  "debug": false
}
```

记得给数据目录适当的权限：
```bash
mkdir -p /var/lib/cmpp-gateway
chmod 755 /var/lib/cmpp-gateway
```

---

## 📚 参考资料

- **BoltDB (bbolt)**: https://github.com/etcd-io/bbolt
- **BoltDB 文档**: https://pkg.go.dev/go.etcd.io/bbolt
- **使用案例**: 
  - etcd (分布式键值存储)
  - Consul (服务发现)
  - InfluxDB (时序数据库)

---

## ✅ 总结

通过这次重构：
1. ✅ **消除外部依赖**：不再需要 Redis
2. ✅ **简化部署**：单个二进制文件即可运行
3. ✅ **保持兼容**：仍支持 Redis（可选）
4. ✅ **纯 Go 实现**：跨平台编译无障碍
5. ✅ **成熟可靠**：使用经过验证的 BoltDB

建议：
- 新部署直接使用 BoltDB
- 现有 Redis 部署可逐步迁移
- 小规模使用 BoltDB 完全足够

---

**如有问题，请查看日志或提交 Issue。**

