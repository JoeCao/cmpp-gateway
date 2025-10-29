# CMPP 3.0 HTTP 网关

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

一个高性能的 CMPP 3.0 协议网关，将中国移动的 CMPP（China Mobile Peer-to-Peer）协议转换为简洁的 HTTP API，为 Web 应用提供便捷的短信收发接口。

## 目录

- [功能特性](#功能特性)
- [系统架构](#系统架构)
- [快速开始](#快速开始)
  - [系统要求](#系统要求)
  - [安装步骤](#安装步骤)
  - [配置说明](#配置说明)
  - [启动服务](#启动服务)
- [API 接口](#api-接口)
- [开发指南](#开发指南)
  - [环境搭建](#环境搭建)
  - [编译构建](#编译构建)
  - [CMPP 模拟器](#cmpp-模拟器)
- [技术架构](#技术架构)
- [依赖管理](#依赖管理)
- [许可证](#许可证)
- [致谢](#致谢)

## 功能特性

- **协议转换**：将复杂的 CMPP 3.0 协议封装为简洁的 HTTP RESTful API
- **高并发处理**：采用 Go 协程实现异步消息处理，单连接多路复用
- **消息追踪**：基于 Redis 实现 SEQID 和 MSGID 的完整追踪链路
- **自动重连**：内置心跳检测和断线重连机制，保障服务稳定性
- **Web 管理界面**：提供消息历史查询和发送状态监控
- **跨平台支持**：支持 Linux、Windows 等多种操作系统

## 系统架构

本网关采用单连接多协程并发模型，最大化利用 CMPP 连接资源：

```
┌─────────────────────────────────────────────────────────────┐
│                      HTTP API Layer                         │
│  (接收 HTTP 请求，提供 Web UI 和 RESTful 接口)                │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   Message Queue (Channel)                   │
│               (异步消息队列，解耦 HTTP 和 CMPP)               │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                  CMPP Gateway Connection                    │
│              (单一 TCP 连接，三个并发协程)                    │
├──────────────┬─────────────────┬────────────────────────────┤
│   Receiver   │     Sender      │      Heartbeat             │
│   (接收应答)  │   (发送请求)     │     (保持连接)              │
└──────┬───────┴────────▲────────┴────────────────────────────┘
       │                │
       ▼                │
┌─────────────────────────────────────────────────────────────┐
│                      Redis Cache                            │
│  • waitseqcache: SEQID → Message 映射 (临时存储)            │
│  • list_message: 已发送消息历史                              │
│  • list_mo: 接收到的上行消息                                 │
└─────────────────────────────────────────────────────────────┘
```

### 核心流程

**消息提交流程（Submit）**：

1. HTTP API 接收发送请求 → 消息入队列 `Messages` channel
2. Sender 协程取出消息 → 调用 `SendReqPkt()` → 获得 `seq_id`
3. 将 `{seq_id: message}` 存入 Redis `waitseqcache` 哈希表
4. Receiver 协程接收 `SubmitRspPkt` → 通过 `seq_id` 从 Redis 查询原始消息
5. 更新消息状态（添加 `MsgId` 和结果）→ 存入 `list_message` 列表

> **为什么需要 Redis？**
> CMPP 协议采用异步设计，Submit Response 中仅包含 `SeqId` 和网关生成的 `MsgId`，不包含原始消息内容。Redis 通过 SEQID 关联请求和响应，实现完整的消息追踪。

## 快速开始

### 系统要求

- **Go 语言环境**：1.21 或更高版本
- **Redis 服务器**：3.0+ 推荐（用于消息状态存储）
- **CMPP 网关**：中国移动提供的 CMPP 3.0 网关（生产环境）或本地模拟器（开发测试）

### 安装步骤

#### 1. 安装 Redis

**Linux 系统**：
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install redis-server

# CentOS/RHEL
sudo yum install redis

# 启动 Redis 服务
sudo systemctl start redis
sudo systemctl enable redis
```

**Windows 系统**：
- 下载 [Redis for Windows](https://github.com/microsoftarchive/redis/releases)
- 推荐将 Redis 安装为 Windows 服务：
  ```cmd
  redis-server --service-install redis.windows.conf --loglevel verbose
  redis-server --service-start
  ```

**macOS 系统**：
```bash
brew install redis
brew services start redis
```

#### 2. 获取网关程序

**方式一：下载预编译版本**

从 [Releases](https://github.com/JoeCao/cmpp-gateway/releases) 页面下载适合您系统的二进制文件。

**方式二：从源码编译**

```bash
git clone https://github.com/JoeCao/cmpp-gateway.git
cd cmpp-gateway
go build -mod=vendor
```

### 配置说明

在程序目录下创建或编辑 `config.json` 文件：

```json
{
  "user": "204221",                    // CMPP 登录用户名（由运营商提供）
  "password": "052932",                // CMPP 登录密码
  "sms_accessno": "1064899104221",     // 接入码（短信发送方号码）
  "service_id": "JSASXW",              // 业务标识（由运营商分配）
  "http_host": "0.0.0.0",              // HTTP 服务监听地址（0.0.0.0 监听所有网卡）
  "http_port": "8000",                 // HTTP 服务端口
  "cmpp_host": "127.0.0.1",            // CMPP 网关 IP 地址
  "cmpp_port": "7891",                 // CMPP 网关端口
  "debug": true,                       // 调试模式（生产环境建议设为 false）
  "redis_host": "127.0.0.1",           // Redis 服务器地址
  "redis_port": "6379",                // Redis 端口
  "redis_password": ""                 // Redis 密码（如未设置密码则留空）
}
```

**⚠️ 安全提醒**：
- 请勿将包含真实凭据的 `config.json` 提交到版本控制系统
- 生产环境建议使用环境变量或加密配置管理工具

### 启动服务

**Linux / macOS**：
```bash
# 使用默认配置文件 config.json
./cmpp-gateway

# 指定自定义配置文件
./cmpp-gateway -c /path/to/config.json
```

**Windows**：
```cmd
REM 双击 cmpp-gateway.exe 或在命令行执行
cmpp-gateway.exe

REM 指定配置文件
cmpp-gateway.exe -c C:\path\to\config.json
```

**验证启动**：

服务启动后，访问 `http://localhost:8000` 查看 Web 管理界面。

## API 接口

### 发送短信

**接口地址**：`GET/POST /submit`

**请求参数**：

| 参数   | 类型   | 必填 | 说明                    |
|--------|--------|------|-------------------------|
| src    | string | 否   | 扩展码（可选）          |
| dest   | string | 是   | 接收手机号（11 位）     |
| cont   | string | 是   | 短信内容（支持中文）    |

**请求示例**：
```bash
# GET 方式
curl "http://localhost:8000/submit?src=test&dest=13800138000&cont=您的验证码是123456"

# POST 方式
curl -X POST "http://localhost:8000/submit" \
  -d "src=test&dest=13800138000&cont=您的验证码是123456"
```

**响应格式**：
```json
{
  "result": 0,        // 0 表示成功，非 0 表示失败
  "error": ""         // 错误信息（成功时为空字符串）
}
```

### 查询消息历史

**已发送消息**：`GET /list_message?page=1`

**上行消息（MO）**：`GET /list_mo?page=1`

### Web 管理界面

访问 `http://localhost:8000/` 查看可视化管理界面，支持：
- 发送短信测试
- 查看消息发送历史
- 查看上行消息
- 实时状态监控

## 开发指南

### 环境搭建

1. **安装 Go 环境**：访问 [golang.org](https://golang.org/dl/) 下载并安装 Go 1.21+
2. **验证安装**：
   ```bash
   go version  # 应显示 go version go1.21 或更高
   ```
3. **克隆仓库**：
   ```bash
   git clone https://github.com/JoeCao/cmpp-gateway.git
   cd cmpp-gateway
   ```

### 编译构建

本项目使用 **Go Modules + Vendor** 模式管理依赖，所有依赖已包含在 `vendor/` 目录中，无需额外下载。

**本地编译**：
```bash
# 默认编译（自动使用 vendor）
go build

# 显式指定 vendor 模式
go build -mod=vendor
```

**交叉编译**：
```bash
# Linux 64 位
GOOS=linux GOARCH=amd64 go build -o cmpp-gateway-linux

# Windows 64 位
GOOS=windows GOARCH=amd64 go build -o cmpp-gateway.exe

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o cmpp-gateway-darwin-arm64

# macOS AMD64 (Intel)
GOOS=darwin GOARCH=amd64 go build -o cmpp-gateway-darwin-amd64
```

### CMPP 模拟器

#### 使用内置模拟器（推荐）

本项目在 `simulator/` 目录提供了开箱即用的 CMPP 3.0 模拟器：

```bash
# 进入模拟器目录
cd simulator

# 编译模拟器
go build -mod=vendor -o cmpp-simulator server.go

# 启动模拟器（默认监听 7890 端口）
./cmpp-simulator
```

**模拟器特性**：
- ✅ 支持 CMPP 3.0 和 CMPP 2.0 协议
- ✅ 自动接受所有连接（无需预配置账号密码）
- ✅ 处理短信提交请求并返回成功响应
- ✅ 支持心跳保活机制
- ✅ 详细的协议交互日志

详细文档：[simulator/README.md](simulator/README.md)

#### 使用外部商业模拟器

如需完整的商业级测试环境，可搜索"CMPP 模拟器"获取第三方工具（通常需要 JVM）。

> **注意**：
> - 模拟器仅用于开发测试，不会真实发送短信
> - 生产环境请连接中国移动提供的正式 CMPP 网关

## 技术架构

### 核心技术栈

- **语言框架**：Go 1.21+ with Go Modules
- **协议实现**：[gocmpp](https://github.com/bigwhite/gocmpp) - CMPP 3.0 协议库
- **缓存存储**：Redis - 消息状态追踪
- **HTTP 服务**：Go 标准库 `net/http`
- **字符编码**：GB18030（中国移动标准）

### 并发模型

采用 **3 协程 + 1 连接** 的异步并发架构：

1. **Receiver 协程**：持续监听 CMPP 网关响应（SubmitRsp、DeliverReq 等）
2. **Sender 协程**：从消息队列获取待发送消息，发送 SubmitReq 请求
3. **Heartbeat 协程**：每 10 秒发送心跳包，检测连接状态并自动重连

**优势**：
- 单连接避免运营商连接数限制
- 异步处理提升吞吐量
- 自动重连保障服务可用性

### 目录结构

```
cmpp-gateway/
├── main.go                 # 主程序入口，启动所有服务
├── config.json            # 运行时配置文件（不提交敏感信息）
├── gateway/               # 核心业务包
│   ├── client.go         # CMPP 连接管理、收发协程
│   ├── cache.go          # Redis 操作封装
│   ├── httpserver.go     # HTTP API 处理器
│   ├── config.go         # 配置加载与解析
│   ├── models.go         # 数据结构定义（SmsMes 等）
│   ├── cmdline.go        # 命令行交互界面
│   └── utils.go          # 工具函数（编码转换等）
├── pages/                # 分页工具包
├── simulator/            # CMPP 模拟器
│   ├── server.go         # 模拟器主程序
│   └── README.md         # 模拟器使用说明
├── vendor/               # 依赖包（已锁定版本）
├── *.html                # Web 管理界面模板
├── CLAUDE.md             # AI 辅助开发指南
├── DEPENDENCIES.md       # 依赖管理详细文档
└── README.md             # 本文件
```

## 依赖管理

本项目采用 **Vendor 模式** 锁定依赖版本，确保构建可重现性。

**主要依赖**：

| 依赖包                          | 版本/提交      | 说明                      |
|---------------------------------|----------------|---------------------------|
| github.com/bigwhite/gocmpp      | 无语义化版本   | CMPP 协议实现（通过 commit 锁定） |
| github.com/garyburd/redigo      | v1.6.4         | Redis 客户端              |
| golang.org/x/text               | v0.14.0        | 字符编码转换              |

**更新依赖**：
```bash
# 更新特定包
go get github.com/bigwhite/gocmpp@<commit-hash>

# 整理依赖
go mod tidy

# 更新 vendor 目录
go mod vendor
```

详细信息请参阅 [DEPENDENCIES.md](DEPENDENCIES.md)。

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。

## 致谢

特别感谢 [@bigwhite](https://github.com/bigwhite) 提供的优秀 CMPP 协议库：
- 📦 [gocmpp](https://github.com/bigwhite/gocmpp) - 坚实可靠的 Go 语言 CMPP 协议实现

---

## 常见问题

<details>
<summary><strong>Q: 为什么使用 vendor 目录？</strong></summary>

A: `gocmpp` 库没有语义化版本标签（semver），通过 vendor 模式锁定特定 commit，确保不同环境编译结果一致，即使上游仓库变更也不受影响。
</details>

<details>
<summary><strong>Q: Redis 连接失败怎么办？</strong></summary>

A: 检查以下几点：
1. Redis 服务是否启动：`redis-cli ping` 应返回 `PONG`
2. 配置文件中的 `redis_host` 和 `redis_port` 是否正确
3. 如果设置了密码，检查 `redis_password` 配置
4. 防火墙是否允许 6379 端口访问
</details>

<details>
<summary><strong>Q: CMPP 连接断开如何处理？</strong></summary>

A: 网关内置自动重连机制，心跳协程检测到连接断开会自动重新建立连接。查看日志确认重连状态，如持续失败，检查：
- CMPP 网关 IP 和端口配置
- 用户名和密码是否正确
- 网络连通性（ping cmpp_host）
</details>

<details>
<summary><strong>Q: 如何在生产环境部署？</strong></summary>

A: 建议部署方式：
1. 使用 systemd/supervisor 等工具管理进程
2. 配置日志轮转避免日志文件过大
3. 关闭 debug 模式（`"debug": false`）
4. 监控 Redis 和 CMPP 连接状态
5. 配置反向代理（如 Nginx）提供 HTTPS 和访问控制
</details>

---

**项目维护者**：[@JoeCao](https://github.com/JoeCao)

如有问题或建议，欢迎提交 [Issue](https://github.com/JoeCao/cmpp-gateway/issues) 或 [Pull Request](https://github.com/JoeCao/cmpp-gateway/pulls)。
