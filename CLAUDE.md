# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **CMPP 3.0 to HTTP Gateway** that converts China Mobile's complex CMPP (China Mobile Peer-to-Peer) 3.0 protocol into a simple HTTP API for SMS sending and receiving. The gateway maintains a persistent connection to a CMPP server and exposes HTTP endpoints for web applications.

**Key Technologies:**
- Go 1.22+ with Go Modules + vendor
- CMPP 3.0 protocol (via github.com/bigwhite/gocmpp)
- Dual cache support: BoltDB (default, embedded) or Redis
- Bootstrap 5.3 Web UI with Go templates
- HTTP API with modern responsive interface

## Build and Run

### Prerequisites

**Choose your cache backend:**

**Option A: BoltDB (Recommended, Default)**
- ✅ Zero external dependencies
- ✅ Single binary deployment
- ✅ Embedded database

**Option B: Redis (Optional)**
- Requires Redis server 3.0+
- For distributed deployments
- Use when already have Redis infrastructure

### Build Commands

```bash
# Standard build (uses vendor automatically)
go build

# Explicit vendor mode
go build -mod=vendor

# Cross-compilation examples
GOOS=linux GOARCH=amd64 go build      # Linux 64-bit
GOOS=windows GOARCH=amd64 go build    # Windows 64-bit
GOOS=darwin GOARCH=arm64 go build     # macOS ARM64
```

### Configuration

**BoltDB Configuration (config.json):**
```json
{
  "user": "204221",
  "password": "052932",
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

**Redis Configuration:**
```json
{
  "cache_type": "redis",
  "redis_host": "127.0.0.1",
  "redis_port": "6379",
  "redis_password": ""
}
```

### Running

```bash
# Use default config.json
./cmpp-gateway

# Specify custom config
./cmpp-gateway -c /path/to/config.json
```

### Development Setup with CMPP Simulator

```bash
# Terminal 1: Start built-in CMPP simulator
cd simulator
go build -mod=vendor -o cmpp-simulator server.go
./cmpp-simulator

# Terminal 2: Start gateway with BoltDB
cd ..
go build -mod=vendor
./cmpp-gateway -c config.boltdb.json

# Test API
curl "http://localhost:8000/submit?src=test&dest=13800138000&cont=Hello"
```

## Architecture

### Concurrency Model

The system uses **3 concurrent goroutines** on a single CMPP connection:

```
                  ┌─────────────────┐
                  │   CMPP Gateway  │
                  └────────┬────────┘
                           │ Single TCP Connection
              ┌────────────┼────────────┐
              │            │            │
         ┌────▼───┐   ┌───▼────┐  ┌───▼────┐
         │Receiver│   │ Sender │  │HeartBeat│
         │GoRoutine│  │GoRoutine│ │GoRoutine│
         └────┬───┘   └───▲────┘  └────────┘
              │           │
              │      Messages chan
              │           │
         ┌────▼───────────┴────┐
         │   HTTP Server       │
         └─────────────────────┘
```

**Key goroutines started in main.go:**
1. `StartClient()` - CMPP connection, starts receiver/sender/heartbeat goroutines
2. `InitCache()` - Cache initialization (BoltDB or Redis)
3. `Serve()` - HTTP server for API and Web UI

### Message Flow: Submit Request

Critical sequence for understanding SEQID tracking:

1. **HTTP Request** → `handler()` receives SMS → adds `SmsMes` to `Messages` channel
2. **Sender goroutine** → reads from channel → `c.SendReqPkt(p)` → gets `seq_id`
3. **Cache Backend** → stores `{seq_id: SmsMes}` in `wait` bucket/hash
4. **Receiver goroutine** → receives `Cmpp3SubmitRspPkt` → looks up `SeqId` in cache
5. **Update** → adds `MsgId` and `SubmitResult` → stores in `messages` bucket/list

**Why this matters**: CMPP is asynchronous - submit response only contains `SeqId` and `MsgId`, not the original message content. Cache bridges this async gap.

### Package Structure

```
cmpp-gateway/
├── main.go                  # Entry point, starts all goroutines
├── config.json             # Runtime configuration (NEVER commit credentials)
├── gateway/                # Core package
│   ├── client.go          # CMPP connection, receiver, sender, heartbeat
│   ├── cache.go           # Cache operations (BoltDB/Redis abstraction)
│   ├── httpserver.go      # HTTP handlers, template rendering
│   ├── config.go          # Config struct and loader
│   ├── models.go          # SmsMes data structure
│   ├── cmdline.go         # Interactive CLI (currently disabled)
│   └── utils.go          # Encoding utilities (GB18030)
├── pages/                 # Pagination helper
│   └── pages.go           # Page struct with pagination logic
├── templates/             # Web UI templates (Bootstrap 5.3)
│   ├── layouts/          # Base layout templates
│   │   └── base.html     # Main layout with navbar, footer, styles
│   ├── partials/         # Reusable components
│   │   ├── navbar.html   # Navigation bar
│   │   └── footer.html   # Page footer
│   └── pages/            # Page-specific content
│       ├── index.html        # Dashboard with stats and send form
│       ├── list_message.html # Submitted messages history
│       └── list_mo.html      # Mobile-originated messages
├── simulator/            # Built-in CMPP 3.0 simulator
└── data/                 # BoltDB data directory (auto-created)
```

## Template System

The project uses modern template inheritance with Bootstrap 5.3:

### Template Loading

Templates are loaded once at startup in `gateway/httpserver.go`:

```go
func initTemplates() error {
    // Parse all templates from layouts/, partials/, pages/
    templates.ParseFiles(allFiles...)
}
```

### Template Naming Convention

**Critical Rule**: Each page must use unique template block names to avoid conflicts.

**Pattern**: `{page_name}_{block_type}`

Examples:
- `index_content` - Home page content
- `index_scripts` - Home page scripts
- `list_message_content` - Submit history content
- `list_mo_scripts` - MO messages scripts

### Conditional Rendering in base.html

The base layout uses `ActivePage` to render the correct content:

```html
{{if eq .ActivePage "home"}}
    {{template "index_content" .}}
{{else if eq .ActivePage "list_message"}}
    {{template "list_message_content" .}}
{{else if eq .ActivePage "list_mo"}}
    {{template "list_mo_content" .}}
{{end}}
```

**Data Structure Contract**: All handlers must pass data with `ActivePage` field:

```go
data := struct {
    ActivePage string  // Required: "home", "list_message", or "list_mo"
    // ... other page-specific fields
}{
    ActivePage: "home",
    // ...
}
```

## HTTP API Endpoints

**Submit SMS:**
```
GET/POST /submit?src={source}&dest={destination}&cont={content}
Response: {"result": 0, "error": ""}
```

**Message History:**
```
GET /list_message?page=1    # Submitted messages
GET /list_mo?page=1          # Mobile-originated (incoming)
```

**Web UI:**
```
GET /                        # Dashboard with stats and send form
```

## Cache Backends

### BoltDB (Default)

**Data Structure:**
- `wait` bucket: `{SEQID} -> {SmsMes JSON}` (temporary, deleted after response)
- `messages` bucket: Submitted message history (newest first)
- `mo` bucket: Mobile-originated message history (newest first)

**Advantages:**
- Zero external dependencies
- Single binary deployment
- ACID transactions
- Cross-platform

### Redis (Optional)

**Data Structure:**
- `waitseqcache` hash: `{SEQID} -> {SmsMes JSON}` (temporary)
- `list_message` list: Submitted message history
- `list_mo` list: Mobile-originated message history

**Advantages:**
- Distributed deployments
- Existing Redis infrastructure
- Advanced data structures

## Configuration

**config.json** structure:

```json
{
  "user": "204221",              // CMPP login username
  "password": "052932",          // CMPP password
  "sms_accessno": "1064899104221", // SMS sender number
  "service_id": "JSASXW",        // Service ID from carrier
  "http_host": "0.0.0.0",        // HTTP bind address
  "http_port": "8000",           // HTTP port
  "cmpp_host": "127.0.0.1",      // CMPP gateway IP
  "cmpp_port": "7891",           // CMPP port
  "debug": true,                 // Verbose logging
  "cache_type": "boltdb",        // "boltdb" (default) or "redis"
  "db_path": "./data/cmpp.db",   // BoltDB file path
  "redis_host": "127.0.0.1",     // Redis IP (if using Redis)
  "redis_port": "6379",          // Redis port
  "redis_password": ""           // Redis password (optional)
}
```

**⚠️ Security**: Never commit real credentials to git.

## Dependency Management

**Critical**: Uses **vendor directory + Go Modules** for strict version locking.

**Key points:**
- `gocmpp` has no semver tags - locked via commit hash
- Dependencies in `vendor/` (committed to git)
- Update: `go get <package>@<version>` → `go mod tidy` → `go mod vendor`
- Build automatically uses vendor (Go 1.14+)

**Main dependencies:**
- `github.com/bigwhite/gocmpp@b238366bff0b` - CMPP 3.0 protocol
- `github.com/gomodule/redigo@v1.9.3` - Redis client (optional)
- `go.etcd.io/bbolt@v1.3.11` - Embedded database (default)

## Code Conventions

### Global State (gateway package)

```go
var Messages = make(chan SmsMes, 10)  // Send queue HTTP → CMPP
var Abort = make(chan struct{})       // Graceful shutdown
var config *Config                    // Runtime config
var c *cmpp.Client                    // Singleton CMPP connection
var SCache CacheInterface             // Cache backend (Redis/BoltDB)
var templates *template.Template      // Preloaded templates
```

### Error Handling

- `log.Printf()` for operational errors (connection drops, protocol errors)
- `log.Fatal()` for fatal startup errors (config load, cache unavailable)
- CMPP errors trigger auto-reconnection via heartbeat goroutine

### CMPP Client API

**Important**: Current API returns both seqId and error:

```go
seq_id, err := c.SendReqPkt(p)  // Returns (uint32, error)
```

## Development Workflow

### Testing with Simulator

1. **Start simulator**: `cd simulator && ./cmpp-simulator`
2. **Start gateway**: `./cmpp-gateway -c config.json`
3. **Test API**: `curl "http://localhost:8000/send?src=test&dest=13800138000&cont=hello"`
4. **Check Web UI**: Open `http://localhost:8000/`

### Adding New Features

1. **New HTTP endpoints**: Add handler in `httpserver.go`, register route in `Serve()`
2. **New message types**: Extend `SmsMes` in `models.go`
3. **Cache operations**: Add to `CacheInterface` in `cache.go`, implement in both backends
4. **New pages**: Create template in `templates/pages/`, update `base.html` conditionals

### Template Development

1. **Understand the structure**: Base layout + partials + page content
2. **Edit the right file**:
   - Common elements (navbar, footer, styles) → `templates/layouts/base.html` or `templates/partials/`
   - Page-specific content → `templates/pages/{page}.html`
3. **Follow naming convention**: Use unique block names (`{page}_{block}`)
4. **Test template loading**: Check logs for "Loaded N template files"

## Common Pitfalls

### 1. Template Naming Conflicts

**Problem**: Using `{{define "content"}}` in multiple page templates causes conflicts.

**Solution**: Use unique names like `{{define "index_content"}}`, `{{define "list_message_content"}}`.

### 2. Missing Template Data Fields

**Problem**: Template references `.Page.TotalRecord` but struct doesn't have this field.

**Solution**: Ensure `pages.Page` struct has all fields used in templates.

### 3. Channel Blocking

**Problem**: `Messages` channel (buffer=10) blocks if CMPP down and HTTP requests continue.

**Solutions**:
- Increase buffer size
- Use non-blocking send with timeout
- Return HTTP error when queue full

### 4. Cache Backend Selection

**Problem**: Forgetting to configure cache type correctly.

**Solution**: Ensure `cache_type` is set to "boltdb" or "redis" in config.json.

## Character Encoding

Chinese SMS requires GB18030 encoding (China Mobile standard). The `golang.org/x/text/encoding/simplifiedchinese` package handles UTF-8 ↔ GB18030 conversions in `utils.go`.

## Performance Considerations

- **Single CMPP Connection**: Maximizes throughput while respecting carrier limits
- **Async Processing**: Non-blocking HTTP responses with background CMPP operations
- **Cache Abstraction**: Switch between BoltDB (embedded) and Redis (distributed) without code changes
- **Template Caching**: Templates loaded once at startup, not per-request

## Deployment

### Production Deployment

1. **Build for target platform**:
   ```bash
   GOOS=linux GOARCH=amd64 go build -mod=vendor -o cmpp-gateway-linux
   ```

2. **Configuration management**:
   - Use environment variables or external config for secrets
   - Set `"debug": false` in production
   - Configure proper log rotation

3. **Process management**:
   ```bash
   # systemd example
   sudo systemctl enable cmpp-gateway
   sudo systemctl start cmpp-gateway
   ```

4. **Monitoring**:
   - Monitor CMPP connection status via heartbeat logs
   - Monitor cache backend health (Redis/BoltDB file size)
   - HTTP endpoint health checks

### BoltDB vs Redis in Production

**Choose BoltDB when:**
- Single server deployment
- Want zero external dependencies
- Simpler operations and backup

**Choose Redis when:**
- Multiple gateway instances
- Need distributed caching
- Already have Redis infrastructure
- Need advanced data structures or TTL

## File Encodings

- **Source code**: UTF-8
- **SMS content**: GB18030 (China Mobile standard)
- **HTTP responses**: UTF-8
- **Templates**: UTF-8

## Documentation Files

- **README.md** - User-facing documentation (installation, API, deployment)
- **CLAUDE.md** - This file (development guide for AI assistants)
- **DEPENDENCIES.md** - Detailed dependency management info
- **TESTING_CHECKLIST.md** - Comprehensive testing scenarios
- **simulator/README.md** - CMPP simulator usage guide
- **BOLTDB_MIGRATION.md** - Migration guide from Redis to BoltDB