# BoltDB 版本快速开始

## 🚀 5分钟快速上手

本指南帮助你快速启动使用 BoltDB 的 CMPP 网关。

---

## 步骤 1：准备配置文件

创建 `config.json`（或使用 `config.boltdb.json`）：

```json
{
  "user": "your_user_id",
  "password": "your_password",
  "sms_accessno": "1069XXXXXXXX",
  "service_id": "YOUR_SERVICE_ID",
  "http_host": "0.0.0.0",
  "http_port": "8000",
  "cmpp_host": "127.0.0.1",
  "cmpp_port": "7891",
  "debug": true,
  "cache_type": "boltdb",
  "db_path": "./data/cmpp.db"
}
```

**注意**：
- 如果不指定 `cache_type`，默认就是 `boltdb`
- 数据目录会自动创建

---

## 步骤 2：启动网关

### 使用预编译版本

```bash
# 直接运行
./cmpp-gateway

# 或指定配置文件
./cmpp-gateway -c config.boltdb.json
```

### 从源码编译运行

```bash
# 克隆项目
git clone https://github.com/JoeCao/cmpp-gateway.git
cd cmpp-gateway

# 下载依赖
go mod tidy

# 编译并运行
go build -mod=mod -o cmpp-gateway
./cmpp-gateway
```

---

## 步骤 3：验证运行

### 查看日志

启动成功后，你应该看到类似的日志：

```
2024/10/29 15:20:00 加载成功 =>  &{...}
2024/10/29 15:20:00 使用 BoltDB 作为缓存后端
2024/10/29 15:20:00 BoltDB 初始化成功: ./data/cmpp.db
2024/10/29 15:20:00 client connect and auth ok
2024/10/29 15:20:00 HTTP服务器启动，监听端口 8000
```

### 访问 Web 界面

打开浏览器访问：http://localhost:8000

你应该能看到：
- 📊 统计信息页面
- 📤 发送消息列表
- 📥 接收消息列表

### 测试 API

```bash
# 发送短信
curl -X POST http://localhost:8000/send \
  -H "Content-Type: application/json" \
  -d '{
    "dest": "13800138000",
    "content": "测试短信"
  }'

# 查看发送列表
curl http://localhost:8000/list
```

---

## 步骤 4：检查数据文件

```bash
# 查看数据目录
ls -lh data/

# 输出示例
# -rw-r--r-- 1 user group  128K Oct 29 15:20 cmpp.db
```

数据会自动保存到 `./data/cmpp.db` 文件中。

---

## 🎯 与 Redis 版本的对比

| 特性 | BoltDB 版本 | Redis 版本 |
|-----|------------|-----------|
| **外部依赖** | ❌ 无需安装 | ✅ 需要 Redis 服务 |
| **配置复杂度** | 🟢 简单 | 🟡 中等 |
| **部署步骤** | 1 步（运行程序） | 2 步（Redis + 程序） |
| **数据持久化** | ✅ 自动 | 🟡 需要配置 |
| **跨平台编译** | ✅ 完全支持 | ✅ 支持 |
| **适用规模** | 中小规模 | 大规模 |

---

## 🔧 常见问题

### 1. 找不到数据文件？

数据文件在首次运行时自动创建。检查：
```bash
ls -la data/
```

如果目录不存在，程序会自动创建。

### 2. 权限错误？

确保程序有权限在当前目录创建 `data` 文件夹：
```bash
chmod +x cmpp-gateway
mkdir -p data
chmod 755 data
```

### 3. 如何备份数据？

BoltDB 使用单文件存储，备份很简单：
```bash
# 停止程序
# 复制数据文件
cp data/cmpp.db data/cmpp.db.backup

# 或带日期
cp data/cmpp.db data/cmpp.db.$(date +%Y%m%d_%H%M%S)
```

### 4. 如何清空历史数据？

```bash
# 停止程序
# 删除数据文件
rm data/cmpp.db
# 重启程序，会自动创建新文件
```

### 5. 可以同时运行多个实例吗？

BoltDB 使用文件锁，同一数据文件不能被多个进程同时打开。

如需运行多个实例，请：
- 使用不同的端口
- 使用不同的数据文件路径

示例：
```json
// 实例 1
{
  "http_port": "8000",
  "db_path": "./data/cmpp1.db"
}

// 实例 2  
{
  "http_port": "8001",
  "db_path": "./data/cmpp2.db"
}
```

---

## 🎓 进阶使用

### 查看数据库内容

安装 bbolt 命令行工具：
```bash
go install go.etcd.io/bbolt/cmd/bbolt@latest
```

使用工具：
```bash
# 查看数据库信息
bbolt info data/cmpp.db

# 查看所有 buckets
bbolt buckets data/cmpp.db

# 输出示例：
# wait
# messages
# mo

# 导出数据
bbolt get data/cmpp.db messages > messages.json
```

### 性能优化

如果消息量很大，考虑：

1. **定期清理旧数据**（未来版本会自动实现）
2. **定期压缩数据库**：
```bash
bbolt compact -o data/cmpp.compact.db data/cmpp.db
mv data/cmpp.compact.db data/cmpp.db
```

---

## 📊 测试建议

### 使用模拟器测试

项目自带 CMPP 模拟器，用于测试：

```bash
# 编译模拟器
cd simulator
go build -o cmpp-simulator server.go

# 启动模拟器（监听 7891 端口）
./cmpp-simulator

# 在另一个终端启动网关
cd ..
./cmpp-gateway
```

详见：[simulator/README.md](simulator/README.md)

---

## ✅ 验证清单

- [ ] 配置文件创建成功
- [ ] 程序启动无错误
- [ ] Web 界面可访问
- [ ] 可以发送测试短信
- [ ] 数据文件已创建
- [ ] 消息历史可查看

全部勾选？恭喜你，已成功部署！🎉

---

## 📚 更多文档

- [完整迁移指南](BOLTDB_MIGRATION.md) - BoltDB 详细说明
- [README](README.md) - 项目完整文档
- [API 文档](README.md#api-接口) - HTTP API 说明

---

## 💡 小贴士

1. **开发环境**：使用 `debug: true` 查看详细日志
2. **生产环境**：使用 `debug: false` 减少日志输出
3. **备份习惯**：定期备份 `data/cmpp.db` 文件
4. **监控建议**：监控数据文件大小，避免无限增长

---

**享受无依赖的简单部署！** 🚀

