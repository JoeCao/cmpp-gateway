# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **CMPP 3.0 to HTTP Gateway** that converts China Mobile's complex CMPP (China Mobile Peer-to-Peer) 3.0 protocol into a simple HTTP API for SMS sending and receiving. The gateway maintains a persistent connection to a CMPP server and exposes HTTP endpoints for web applications.

**Key Technologies:**
- Go 1.21+ with Go Modules + vendor
- CMPP 3.0 protocol (via github.com/bigwhite/gocmpp)
- Redis for message state tracking
- Bootstrap 5.3 Web UI with Go templates
- HTTP API with modern responsive interface

## Build and Run

### Build Commands

```bash
# Standard build (uses vendor automatically)
go build

# Explicit vendor mode
go build -mod=vendor

# Cross-compilation examples
GOOS=linux GOARCH=amd64 go build      # Linux 64-bit
GOOS=windows GOARCH=amd64 go build    # Windows 64-bit
```

### Running

```bash
# Use default config.json
./cmpp-gateway

# Specify custom config
./cmpp-gateway -c /path/to/config.json
```

### Prerequisites

**Must be running before starting:**
1. **Redis** - For message state tracking and history
   ```bash
   redis-server
   redis-cli ping  # Should return PONG
   ```

2. **CMPP Gateway** - Real carrier gateway or simulator
   - For development: Use built-in simulator in `simulator/` directory
   - See `simulator/README.md` for simulator usage

## Architecture

### Concurrency Model

The system uses **3 concurrent goroutines** on a single CMPP connection to maximize throughput:

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
2. `StartCache()` - Redis connection initialization
3. `Serve()` - HTTP server for API and Web UI
4. `StartCmdLine()` - Command-line interface (optional)

### Message Flow: Submit Request

Critical sequence for understanding SEQID tracking:

1. **HTTP Request** → `handler()` receives SMS → adds `SmsMes` to `Messages` channel
2. **Sender goroutine** → reads from channel → `c.SendReqPkt(p)` → gets `seq_id`
3. **Redis Cache** → stores `{seq_id: SmsMes}` in `waitseqcache` hash
4. **Receiver goroutine** → receives `Cmpp3SubmitRspPkt` → looks up `SeqId` in Redis
5. **Update** → adds `MsgId` and `SubmitResult` → stores in `list_message`

**Why this matters**: CMPP is asynchronous - submit response only contains `SeqId` and `MsgId`, not the original message content. Redis bridges this async gap.

### Package Structure

```
cmpp-gateway/
├── main.go                  # Entry point, starts all goroutines
├── config.json             # Runtime configuration (NEVER commit credentials)
├── gateway/                # Core package
│   ├── client.go          # CMPP connection, receiver, sender, heartbeat
│   ├── cache.go           # Redis operations (SEQID→Message mapping)
│   ├── httpserver.go      # HTTP handlers, template rendering
│   ├── config.go          # Config struct and loader
│   ├── models.go          # SmsMes data structure
│   ├── cmdline.go         # Interactive CLI
│   └── utils.go           # Encoding utilities (GB18030)
├── pages/                 # Pagination helper
│   └── pages.go           # Page struct with pagination logic
├── templates/             # Web UI templates (NEW: Bootstrap 5)
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
└── *.html               # Legacy templates (root directory, deprecated)
```

## Template System Architecture

**IMPORTANT**: The project uses a modern template inheritance system (as of 2025-10-29).

### Template Loading

Templates are loaded once at startup in `gateway/httpserver.go`:

```go
func initTemplates() error {
    // Define custom template functions
    funcMap := template.FuncMap{
        "add": func(a, b int) int { return a + b },
        "sub": func(a, b int) int { return a - b },
        "mul": func(a, b int) int { return a * b },
        "eq":  func(a, b interface{}) bool { return a == b },
        "pageRange": func(current, total int) []int { /* ... */ },
    }

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
  "redis_host": "127.0.0.1",     // Redis IP
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

See `DEPENDENCIES.md` for full details.

## Code Conventions

### Global State (gateway package)

```go
var Messages = make(chan SmsMes, 10)  // Send queue HTTP → CMPP
var Abort = make(chan struct{})       // Graceful shutdown
var config *Config                    // Runtime config
var c *cmpp.Client                    // Singleton CMPP connection
var SCache Cache                      // Singleton Redis connection
var templates *template.Template      // Preloaded templates
```

### Error Handling

- `log.Printf()` for operational errors (connection drops, protocol errors)
- `log.Fatal()` for fatal startup errors (config load, Redis unavailable)
- CMPP errors trigger auto-reconnection via heartbeat goroutine

### CMPP Client API

**Important**: Current API returns both seqId and error:

```go
seq_id, err := c.SendReqPkt(p)  // Returns (uint32, error)
```

## HTTP API Endpoints

**Submit SMS:**
```
GET/POST /send?src={source}&dest={destination}&cont={content}
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

## Development Workflow

### Modifying Web UI Templates

1. **Understand the structure**: Base layout + partials + page content
2. **Edit the right file**:
   - Common elements (navbar, footer, styles) → `templates/layouts/base.html` or `templates/partials/`
   - Page-specific content → `templates/pages/{page}.html`
3. **Follow naming convention**: Use unique block names (`{page}_{block}`)
4. **Test template loading**: Check logs for "Loaded N template files"
5. **If templates fail to load**: System falls back to old `*.html` files in root

**Common template tasks:**
- Add new page: Create `templates/pages/newpage.html` with `{{define "newpage_content"}}`, update `base.html` conditionals, add handler in `httpserver.go`
- Modify navbar: Edit `templates/partials/navbar.html`
- Add custom function: Update `funcMap` in `initTemplates()`

### Modifying CMPP Client Logic

1. Read `client.go` - understand 3 goroutines: `activeTimer`, `startReceiver`, `startSender`
2. CMPP protocol packets in `vendor/github.com/bigwhite/gocmpp`
3. Test with simulator before real gateway
4. Pay attention to `SeqId` tracking - the async correlation ID

### Modifying HTTP API

1. Add handler in `httpserver.go`
2. Register route in `Serve()` function
3. For new message types, extend `SmsMes` in `models.go`
4. Update Redis storage in `cache.go` if persistence needed
5. If adding a new page, follow template system conventions

### Testing

**Manual testing workflow:**

1. Start Redis: `redis-server`
2. Start simulator: `cd simulator && go run server.go`
3. Run gateway: `./cmpp-gateway -c config.json`
4. Test API: `curl "http://localhost:8000/send?src=test&dest=13800138000&cont=hello"`
5. Check Web UI: Open `http://localhost:8000/`
6. Verify Redis: `redis-cli HGETALL waitseqcache`

**See `TESTING_CHECKLIST.md` for comprehensive test scenarios.**

### Cross-Compilation

```bash
# Linux server deployment
GOOS=linux GOARCH=amd64 go build -o cmpp-gateway-linux

# Windows deployment
GOOS=windows GOARCH=amd64 go build -o cmpp-gateway.exe
```

## Redis Data Structures

- `waitseqcache` - Hash: `SEQID` → `SmsMes` JSON (temporary, deleted after response)
- `list_message` - List: Submitted message history (newest first)
- `list_mo` - List: Mobile-originated message history (newest first)

## Key Algorithms

### SEQID Correlation

CMPP Submit/Response are asynchronous:
- **Submit Request**: Contains message content, gets auto-incrementing `SeqId`
- **Submit Response**: Only contains `SeqId` + `MsgId` (no content)
- **Redis bridges the gap**: Store `{SeqId: message}` before send, retrieve on response

### Heartbeat & Reconnection

- Every 10 seconds: Send `CmppActiveTestReqPkt`
- On failure: `connectServer()` + restart `startReceiver()`
- Receiver exits on errors to prevent CPU spin

## Common Pitfalls

### 1. Template Naming Conflicts

**Problem**: Using `{{define "content"}}` in multiple page templates causes conflicts.

**Solution**: Use unique names like `{{define "index_content"}}`, `{{define "list_message_content"}}`.

**See**: `BUG_FIXES.md` for detailed explanation.

### 2. Missing Template Data Fields

**Problem**: Template references `.Page.TotalRecord` but struct doesn't have this field.

**Solution**: Ensure `pages.Page` struct has all fields used in templates. Check struct definition before template changes.

### 3. Channel Blocking

**Problem**: `Messages` channel (buffer=10) blocks if CMPP down and HTTP requests continue.

**Solutions**:
- Increase buffer size
- Use non-blocking send with timeout
- Return HTTP error when queue full

### 4. Character Encoding

**Problem**: Chinese text garbled.

**Solution**: SMS uses GB18030 encoding. See `utils.go` for conversion helpers.

## File Encodings

Chinese SMS requires GB18030 encoding (China Mobile standard). The `golang.org/x/text/encoding/simplifiedchinese` package handles UTF-8 ↔ GB18030 conversions.

## Documentation Files

- **README.md** - User-facing documentation (installation, API, deployment)
- **CLAUDE.md** - This file (development guide for AI assistants)
- **DEPENDENCIES.md** - Detailed dependency management info
- **UI_REFACTORING.md** - Complete UI redesign documentation (Bootstrap 5 upgrade)
- **BUG_FIXES.md** - Template system bug fixes and solutions
- **TESTING_CHECKLIST.md** - Comprehensive testing scenarios
- **simulator/README.md** - CMPP simulator usage guide

## Recent Major Changes

### UI Refactoring (2025-10-29)

- Upgraded from Bootstrap 3 to Bootstrap 5.3
- Implemented template inheritance system with layouts/partials/pages structure
- Added modern dashboard with real-time statistics
- Introduced responsive design for mobile devices
- Added custom template functions for pagination and math operations
- Implemented conditional rendering based on `ActivePage` field

**Key Migration Notes**:
- Old `*.html` files in root still exist for backward compatibility
- New templates in `templates/` directory take precedence
- System falls back to old templates if new ones fail to load
- Each page requires unique template block names to avoid conflicts

**See UI_REFACTORING.md and BUG_FIXES.md for detailed migration guide.**
