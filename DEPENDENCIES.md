# 依赖管理说明

## 当前依赖版本

本项目使用 Go Modules + vendor 目录来确保依赖的稳定性和可重复构建。

### 核心依赖

| 依赖包 | 版本 | 说明 |
|-------|------|------|
| github.com/bigwhite/gocmpp | b238366bff0b (2024-09-17) | CMPP 3.0 协议实现 |
| github.com/gomodule/redigo | v1.9.3 | Redis 客户端 |
| golang.org/x/text | v0.3.8 | 文本编码支持（间接依赖） |

## 为什么使用 vendor？

1. **版本锁定**：`gocmpp` 仓库没有发布正式版本标签，使用 vendor 可以完全控制依赖版本
2. **离线构建**：不依赖外部网络即可构建项目
3. **防止破坏性更新**：上游更新不会影响项目的稳定性
4. **合规性**：某些企业环境要求所有依赖都在仓库中

## 版本锁定机制

本项目采用三层版本锁定：

1. **go.mod**: 指定依赖的语义化版本或伪版本
2. **go.sum**: 记录依赖的校验和（checksum），防止篡改
3. **vendor/**: 实际的依赖代码副本

即使 GitHub 上的依赖仓库发生变化，只要我们不主动更新，项目使用的依赖版本永远不变。

## 如何构建项目

### 使用 vendor（推荐）
```bash
# 自动使用 vendor 目录中的依赖
go build -mod=vendor

# Go 1.14+ 会自动检测 vendor 目录，也可以直接：
go build
```

### 不使用 vendor
```bash
# 从网络下载依赖（需要网络连接）
go build -mod=mod
```

## 如何更新依赖

### 更新特定依赖
```bash
# 1. 更新到最新版本
go get -u github.com/gomodule/redigo

# 2. 或指定具体版本
go get github.com/gomodule/redigo@v1.9.2

# 3. 整理依赖
go mod tidy

# 4. 更新 vendor 目录
go mod vendor

# 5. 测试构建
go build

# 6. 提交更改
git add go.mod go.sum vendor/
git commit -m "Update dependencies"
```

### 更新 gocmpp 到特定提交
```bash
# 使用 commit hash（前 12 位）
go get github.com/bigwhite/gocmpp@b238366bff0b

# 或使用完整 hash
go get github.com/bigwhite/gocmpp@b238366bff0b66ceca1e21ee52d71846946725dc

# 然后更新 vendor
go mod tidy
go mod vendor
```

### 查看可用版本
```bash
# 查看已发布的版本
go list -m -versions github.com/gomodule/redigo

# 查看依赖的当前版本信息
go list -m all
```

## 替代方案

如果不想使用 vendor，可以考虑：

### 方案 1: Fork 依赖仓库
```bash
# 1. Fork github.com/bigwhite/gocmpp 到你的账号
# 2. 修改 go.mod，使用你的 fork
go mod edit -replace github.com/bigwhite/gocmpp=github.com/YourName/gocmpp@v1.0.0

# 3. 在你的 fork 中打版本标签
git tag v1.0.0
git push origin v1.0.0
```

### 方案 2: 使用 replace 指令锁定提交
```go
// 在 go.mod 中添加
replace github.com/bigwhite/gocmpp => github.com/bigwhite/gocmpp v0.0.0-20240917054108-b238366bff0b
```

### 方案 3: 仅依赖 go.sum（最简单但不推荐）
- 只要 `go.sum` 存在，Go 会验证下载的依赖是否匹配
- 但如果 GitHub 仓库删除或不可访问，则无法构建

## 最佳实践建议

✅ **推荐做法**：
- 使用 vendor 目录（当前配置）
- 定期检查依赖更新，但手动升级
- 每次升级后充分测试
- 提交 vendor 目录到版本控制

⚠️ **注意事项**：
- 不要使用 `go get -u all` 盲目更新所有依赖
- 更新依赖前先查看 CHANGELOG 或 commit 历史
- 在非生产环境充分测试后再更新生产环境

## 依赖安全

定期检查依赖的安全漏洞：
```bash
# 安装 govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest

# 检查漏洞
govulncheck ./...
```

## 问题排查

### 依赖下载失败
```bash
# 使用国内代理
go env -w GOPROXY=https://goproxy.cn,direct

# 或使用官方代理
go env -w GOPROXY=https://proxy.golang.org,direct
```

### vendor 不一致
```bash
# 清理并重新生成
rm -rf vendor/
go mod tidy
go mod vendor
```

### 验证依赖完整性
```bash
# 验证 go.sum
go mod verify
```
