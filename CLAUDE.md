# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **CMPP 3.0 to HTTP Gateway** that converts China Mobile's complex CMPP (China Mobile Peer-to-Peer) 3.0 protocol into a simple HTTP API for SMS sending and receiving. The gateway maintains a persistent connection to a CMPP server and exposes HTTP endpoints for web applications.

**Key Technologies:**
- Go 1.21+ with Go Modules + vendor
- CMPP 3.0 protocol (via github.com/bigwhite/gocmpp)
- Redis for message state tracking
- HTTP API with basic web UI

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

### Testing with CMPP Simulator

For development/testing without a real CMPP gateway:
1. Use the CMPP simulator from vendor: `vendor/github.com/bigwhite/gocmpp/examples/server/server.go`
2. Or download full simulator from: http://www.simpleteam.com/doku.php?id=message:cmpp_simulator
   - Requires JVM
   - Configure `config.xml`: set `message="-1"` and `passive="-1"` to avoid heartbeat timeouts

## Architecture

### Concurrency Model

The system uses **3 concurrent goroutines** on a single CMPP connection to maximize throughput while respecting CMPP connection limits:

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
1. `StartClient()` - Connects to CMPP, starts receiver & heartbeat goroutines, processes send queue
2. `StartCache()` - Initializes Redis connection
3. `Serve()` - HTTP server for API and web UI
4. `StartCmdLine()` - Command-line interface (optional)

### Message Flow: Submit Request

Critical sequence for understanding SEQID tracking:

1. **HTTP Request** → `handler()` receives SMS parameters → adds `SmsMes` to `Messages` channel
2. **Sender goroutine** reads from channel → calls `c.SendReqPkt(p)` → gets `seq_id` back
3. **Redis Cache** → stores `seq_id` → `SmsMes` mapping in `waitseqcache` hash
4. **Receiver goroutine** → receives `Cmpp3SubmitRspPkt` → looks up `SeqId` in Redis → retrieves original message
5. **Update** → adds `MsgId` and `SubmitResult` → stores in `list_message` for history

**Why this matters**: CMPP is asynchronous - the submit response doesn't contain the original message content, only the `SeqId`. Redis bridges the async gap.

### Package Structure

```
cmpp-gateway/
├── main.go                 # Entry point, starts all goroutines
├── config.json            # Runtime configuration (NEVER commit credentials)
├── gateway/               # Core package
│   ├── client.go         # CMPP connection, receiver, sender, heartbeat
│   ├── cache.go          # Redis operations (SEQID→Message mapping)
│   ├── httpserver.go     # HTTP handlers (/submit, /list endpoints)
│   ├── config.go         # Config struct and loader
│   ├── models.go         # SmsMes data structure
│   ├── cmdline.go        # Interactive CLI
│   └── utils.go          # Encoding utilities
├── pages/                # Pagination helper
└── *.html                # Web UI templates (served from root)
```

## Configuration

**config.json** structure (see README.md for full example):

```json
{
  "user": "204221",              // CMPP login username
  "password": "052932",          // CMPP login password
  "sms_accessno": "1064899104221", // SMS sender number shown to recipients
  "service_id": "JSASXW",        // Business service ID from carrier
  "http_host": "0.0.0.0",        // HTTP bind address
  "http_port": "8000",           // HTTP port
  "cmpp_host": "127.0.0.1",      // CMPP gateway IP
  "cmpp_port": "7891",           // CMPP gateway port
  "debug": true,                 // Enable verbose logging
  "redis_host": "127.0.0.1",     // Redis IP
  "redis_port": "6379"           // Redis port
}
```

**⚠️ Security**: `config.json` contains credentials. Never commit real credentials to git.

## Dependency Management

**Critical**: This project uses **vendor directory + Go Modules** for strict version locking. See `DEPENDENCIES.md` for full details.

**Key points:**
- `gocmpp` has no semver tags - locked via commit hash in vendor
- Dependencies are in `vendor/` directory (committed to git)
- To update dependencies: `go get <package>@<version>` → `go mod tidy` → `go mod vendor`
- Build automatically uses vendor (Go 1.14+)

**Why vendor?** Ensures reproducible builds even if upstream repos change/disappear.

## Code Conventions

### Global State (in gateway package)

```go
var Messages = make(chan SmsMes, 10)  // Send queue from HTTP → CMPP
var Abort = make(chan struct{})       // Graceful shutdown signal
var config *Config                    // Runtime configuration
var c *cmpp.Client                    // Singleton CMPP connection
var SCache Cache                      // Singleton Redis connection
```

These are package-level singletons initialized by `Start*()` functions.

### Error Handling

- Use `log.Printf()` for operational errors (connection drops, protocol errors)
- Use `log.Fatal()` for fatal startup errors (config load, Redis unavailable)
- CMPP connection errors trigger automatic reconnection via heartbeat goroutine

### CMPP Client Usage

**Important API change** (migrated from old gocmpp):
```go
// OLD (vendor version)
seq_id := c.SendReqPktWithSeqId(p)

// NEW (current version)
seq_id, err := c.SendReqPkt(p)  // Returns (uint32, error)
```

When working with CMPP code, remember `SendReqPkt` now returns both seqId and error.

## HTTP API Endpoints

**Submit SMS:**
```
GET/POST /submit?src={source}&dest={destination}&cont={content}
Response: {"result": 0, "error": ""}
```

**View Message History:**
```
GET /list_message?page=1    # Submitted messages
GET /list_mo?page=1          # Mobile-originated (incoming) messages
```

**Web UI:**
```
GET /                        # Dashboard (index.html)
```

## Development Workflow

### Making Changes to CMPP Client Logic

1. Read `client.go` - understand the 3 goroutines (activeTimer, startReceiver, startSender)
2. CMPP protocol packets are in `github.com/bigwhite/gocmpp` - see vendor for reference
3. Test with CMPP simulator before real gateway
4. Pay attention to `SeqId` tracking - it's the async correlation ID

### Modifying HTTP API

1. Add handler in `httpserver.go`
2. Register route in `Serve()` function
3. For new message types, extend `SmsMes` struct in `models.go`
4. Update Redis storage in `cache.go` if persistence needed

### Testing

Currently **no automated tests** exist. Manual testing steps:

1. Start Redis: `redis-server`
2. Start CMPP simulator or use test gateway
3. Run gateway: `./cmpp-gateway -c config.json`
4. Send test message: `curl "http://localhost:8000/submit?src=test&dest=13800138000&cont=hello"`
5. Check logs for CMPP exchange
6. Verify Redis: `redis-cli HGETALL waitseqcache`

### Cross-Compilation for Deployment

```bash
# For Linux server deployment
GOOS=linux GOARCH=amd64 go build -o cmpp-gateway-linux

# For Windows deployment
GOOS=windows GOARCH=amd64 go build -o cmpp-gateway.exe
```

## Runtime Dependencies

**Must be running before starting gateway:**
1. **Redis** - Used for SEQID tracking and message history
2. **CMPP Gateway** - Mobile carrier's CMPP 3.0 server (or simulator for dev)

**Redis data structures used:**
- `waitseqcache` - Hash: SEQID → SmsMes JSON (temporary, deleted after response)
- `list_message` - List: Submitted message history (newest first)
- `list_mo` - List: Mobile-originated message history

## Key Algorithms

### SEQID Correlation (See README diagram)

CMPP Submit and Submit Response are asynchronous:
- **Submit Request** contains message content, gets assigned auto-incrementing `SeqId`
- **Submit Response** only contains `SeqId` and generated `MsgId` - no content
- Redis bridges this: store `{SeqId: message}` before send, retrieve when response arrives

### Heartbeat & Reconnection

- Every 10 seconds: send `CmppActiveTestReqPkt` to keep connection alive
- If heartbeat fails: `connectServer()` + restart `startReceiver()` goroutine
- Receiver goroutine exits on connection errors to prevent CPU spin

## Common Pitfall

**Channel blocking**: `Messages` channel has buffer of 10. If CMPP connection is down and HTTP keeps receiving requests, channel will block. Consider implementing:
- Larger buffer
- Non-blocking send with timeout
- HTTP error response when queue is full

## File Encodings

Chinese SMS uses GB18030 encoding (see `utils.go`). The `golang.org/x/text/encoding/simplifiedchinese` package handles conversions.
