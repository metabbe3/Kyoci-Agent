<p align="center">
  <h1 align="center">⚡ Kyoci Agent</h1>
  <p align="center">
    <strong>Production-Grade Autonomous AI Agent Engine</strong><br>
    <em>Written in Go · 25MB Binary · 30K+ Lines · Zero Bloat</em>
  </p>
</p>

---

## 🚀 What is Kyoci Agent?

Kyoci Agent is a **high-performance, self-improving AI agent engine** built in pure Go. It combines a ReAct agent loop, hybrid code intelligence (AST + Vector + LSP), multi-tier routing, and MCP (Model Context Protocol) support into a single 25MB binary with zero runtime bloat.

**Think of it as:** Your own private, self-hosted Claude Code / Cursor agent — but faster, lighter, and fully under your control.

---

## ✨ Key Features

### 🧠 Intelligence
- **ReAct Agent Loop** — Think → Act → Observe with auto-compaction
- **4-Phase Step-by-Step Reasoning** — Deconstruct → Plan → Self-Correct → Synthesize
- **Complexity Classifier** — Level 1-5 automatic task classification
- **Plan Mode** — Generate plan → user approve/reject before execution
- **Clarification System** — Auto-detect ambiguity, ask follow-up questions
- **Sub-Agent Delegation** — Spawn isolated sub-agents for parallel tasks (max 3)
- **Self-Improvement** — Learn from execution history, auto-generate new skills

### 🧬 Code Intelligence (Self-Aware)
- **AST Parser** — Parse all `.go` files, extract functions/structs/interfaces/call graph
- **TF-IDF Vector Search** — Semantic code search (zero external DB)
- **gopls/LSP Integration** — Definition, References, Hover, Symbols, Implementations
- **Impact Analysis** — Risk level LOW→CRITICAL, affected services detection
- **Auto-Reindex** — Watch file changes, rebuild index every 30s

### 🔗 MCP (Model Context Protocol)
- **Tools** — Connect to 100+ community MCP servers (GitHub, Postgres, Fetch, etc.)
- **Prompts** — Fetch prompt templates from MCP servers dynamically
- **Resources** — Read live context (API docs, DB schemas, logs) from MCP servers
- **Stdio + SSE Transport** — JSON-RPC 2.0 over subprocess or HTTP

### 🛣️ Production Routing
- **Tier 0** — Zero-AI code execution (< 1ms, 0 tokens)
- **Tier 1** — Local Ollama only (free, fast)
- **Tier 2** — Cloud API only (powerful, paid)
- **Circuit Breaker** — Per-provider, auto-fallback on failure
- **DAG Executor** — Parallel/sequential multi-step plan execution
- **WAL Durable Execution** — Crash recovery, resume from checkpoint
- **Asymmetric Routing** — Local for cheap tasks, cloud for complex ones

### 🛡️ Security
- **API Key Auth** — SHA256 prefix match
- **Rate Limiter** — Sliding window token bucket
- **TLS 1.2-1.3** — With self-signed dev cert generation
- **Input Sanitizer** — Path traversal prevention
- **Docker Sandbox** — Isolated containers (no network, read-only, resource limits)

### 💾 Memory
- **Short-term** — Rolling buffer with token budget
- **Long-term** — SQLite FTS5 full-text search (instant recall)
- **Auto-Compaction** — Compress when approaching token limits

### 🔍 Observability
- **Structured Logging** — `log/slog` JSON output (Datadog/Kibana/Grafana ready)
- **Distributed Tracing** — TraceID propagation across HTTP/gRPC/WebSocket
- **Graceful Shutdown** — SIGTERM handler with 10s drain timeout

### 🔄 Self-Improvement Pipeline
- **Deterministic Validator** — `go vet` + `go build` + `go test` + `golangci-lint`
- **Sandbox Execution** — Safe code modification in temp directory
- **7-Phase Pipeline** — Search → Plan → Execute → Validate → Review → PR → Complete
- **Auto PR** — Branch, commit, push, create PR via `gh` CLI

---

## 📁 Architecture

```
kyoci-agent/
├── agent/           ReAct loop + Sub-Agent delegation + Orchestrator
├── api/             HTTP REST v2 + SSE streaming + WebSocket
├── classifier/      Level 1-5 complexity classification
├── codegraph/       AST + TF-IDF Vector + LSP + Impact Analysis
├── config/          YAML config + env override
├── delegation/      Caveman 5-level token optimization
├── engine/          Unified task pipeline (HTTP/WS/gRPC/REPL → EngineTask)
├── gateway/         Circuit breaker + Tiered router + DAG executor + WAL
├── grpc/            gRPC server + client
├── llm/             4 providers (OpenAI, Anthropic, Ollama, Google) + caching
├── mcp/             MCP Client (Tools + Prompts + Resources)
├── memory/          SQLite FTS5 + JSON fallback + auto-compaction
├── pool/            Bounded worker pool + buffer pool
├── proto/           Protobuf definitions (agent.proto)
├── sandbox/         Docker + SSH + Modal + Container Pool
├── security/        Auth + Rate limit + TLS + Sanitizer
├── selfimprove/     Validator + Sandbox + 7-Phase Pipeline + Auto PR
├── selfskill/       Auto skill creation from patterns
├── skill/           8 zero-AI handlers (math, time, hash, uuid, etc.)
├── thinking/        Step-by-step reasoning + Plan mode + Clarification
├── tools/           15 built-in tools (web, terminal, code, DB, etc.)
└── tracing/         OTel-compatible distributed tracing
```

**Stats:** 30,907 lines Go · 25 packages · 25MB binary · 5 direct deps

---

## 🏗️ Architecture Flow

```
User Request
    │
    ├─ HTTP REST ──┐
    ├─ WebSocket ──┤
    ├─ gRPC ───────┤
    └─ REPL ───────┘
          │
          ▼
    EngineTask (uniform internal format)
          │
          ▼
    ┌─ Tier 0: Zero-AI Skill? ──→ ⚡ INSTANT (<1ms, 0 tokens)
    │
    ├─ Classify (Level 1-5)
    │
    ├─ Level 1-3: Tier 1 → Local Ollama (free, 2.5min timeout)
    │
    └─ Level 4-5: Tier 2 → Cloud API (5min timeout)
           ├─ Step-by-Step Reasoning
           ├─ Plan Mode → user approve/reject
           ├─ Sub-Agent Delegation (parallel)
           ├─ Code context injection (AST + LSP)
           └─ MCP tools/prompts/resources
```

---

## ⚡ Quick Start

### Prerequisites
- Go 1.23+
- (Optional) Ollama for local AI
- (Optional) Docker for sandboxed execution

### Build & Run

```bash
# Clone
git clone https://github.com/metabbe3/Kyoci-Agent.git
cd Kyoci-Agent

# Build
go build -o bin/kyoci .

# Run REPL (interactive mode)
./bin/kyoci --provider ollama

# Single prompt
./bin/kyoci --provider ollama --prompt "analyze this codebase"

# HTTP API server
./bin/kyoci --provider ollama --serve --port 8080

# gRPC server
./bin/kyoci --provider ollama --grpc --grpc-port 50051
```

### Configuration

Copy and edit the default config:

```bash
cp config/config.yaml config/local.yaml
```

Edit `config/local.yaml`:

```yaml
agent:
  name: "Kyoci Agent"
  version: "4.3.0"

providers:
  ollama:
    enabled: true
    base_url: "http://localhost:11434"
    model: "qwen3:8b"
  openai:
    enabled: true
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
  anthropic:
    enabled: true
    api_key: "${ANTHROPIC_API_KEY}"
    model: "claude-sonnet-4-20250514"
  google:
    enabled: true
    api_key: "${GOOGLE_API_KEY}"
    model: "gemini-2.0-flash"

routing:
  tier_bindings:
    tier0: "builtin"
    tier1: "ollama"
    tier2: "openai"
    tier1_fallback: "reject"
    tier2_fallback: "anthropic"

sandbox:
  mode: "auto"   # auto, docker, ssh, host
  docker:
    enabled: true
    pool_size: 3

memory:
  backend: "sqlite"
  db_path: "data/memory.db"
  fts_enabled: true

mcp:
  servers:
    - name: "fetch"
      command: "uvx"
      args: ["mcp-server-fetch"]
    - name: "github"
      command: "npx"
      args: ["@anthropic/mcp-server-github"]
      env:
        GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

---

---

## 🔗 MCP Integration Guide

### Connecting to MCP Servers

Kyoci Agent supports the **Model Context Protocol** for dynamic tool discovery. Add MCP servers to your config:

```yaml
mcp:
  servers:
    # GitHub integration
    - name: "github"
      command: "npx"
      args: ["-y", "@modelcontextprotocol/server-github"]
      env:
        GITHUB_PERSONAL_ACCESS_TOKEN: "${GITHUB_TOKEN}"

    # PostgreSQL database
    - name: "postgres"
      command: "uvx"
      args: ["mcp-server-postgres", "postgresql://user:pass@localhost:5432/mydb"]

    # Web fetching
    - name: "fetch"
      command: "uvx"
      args: ["mcp-server-fetch"]

    # Filesystem access
    - name: "filesystem"
      command: "npx"
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/project"]

    # Brave Search
    - name: "brave-search"
      command: "npx"
      args: ["-y", "@anthropic/mcp-server-brave-search"]
      env:
        BRAVE_API_KEY: "${BRAVE_API_KEY}"
```

### MCP Three Pillars

```go
// 1. TOOLS — Execute actions
tools, _ := mcpClient.ListTools(ctx)
// → [{"name": "create_issue", "description": "Create GitHub issue", ...}]

result, _ := mcpClient.CallTool(ctx, "create_issue", map[string]interface{}{
    "title": "Bug: auth timeout",
    "body":  "Users reporting 503...",
})

// 2. PROMPTS — Fetch prompt templates
prompts, _ := mcpClient.ListPrompts(ctx)
// → [{"name": "review_code_golang", "arguments": [...]}]

prompt, _ := mcpClient.GetPrompt(ctx, "review_code_golang", map[string]string{
    "strict_mode": "true",
})
agent.Memory.Append(prompt.Messages...)

// 3. RESOURCES — Read live context
resources, _ := mcpClient.ListResources(ctx)
// → [{"uri": "db://schema/users", "name": "Users Table Schema"}]

content, _ := mcpClient.ReadResource(ctx, "db://schema/users")
// → Column definitions, indexes, constraints...
```

---

## 🐳 Docker

### Build

```bash
docker build -t kyoci-agent .
```

### Run

```bash
docker run -d \
  --name kyoci \
  -p 8080:8080 \
  -p 50051:50051 \
  -v $(pwd)/config:/app/config \
  -v $(pwd)/data:/app/data \
  -e OPENAI_API_KEY=sk-... \
  kyoci-agent
```

### Docker Compose

```yaml
version: "3.8"
services:
  kyoci:
    build: .
    ports:
      - "8080:8080"
      - "50051:50051"
    volumes:
      - ./config:/app/config
      - ./data:/app/data
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OLLAMA_BASE_URL=http://ollama:11434
    depends_on:
      - ollama

  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama-data:/root/.ollama

volumes:
  ollama-data:
```

---

## 🔧 REST API v2

### POST /v2/chat — Non-streaming chat

```bash
curl -X POST http://localhost:8080/v2/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "analyze this codebase for security issues",
    "session_id": "optional-session-id",
    "mode": "auto",
    "model": "gpt-4o",
    "max_tokens": 4096,
    "temperature": 0.7
  }'
```

**Response:**
```json
{
  "message": "I found 3 security issues...",
  "model": "gpt-4o",
  "session_id": "optional-session-id",
  "tier": 2,
  "tokens": 1247
}
```

### POST /v2/stream — SSE streaming

```bash
curl -N -X POST http://localhost:8080/v2/stream \
  -H "Content-Type: application/json" \
  -d '{
    "message": "explain the architecture",
    "session_id": "optional-session-id",
    "mode": "auto",
    "model": "qwen3:8b"
  }'
```

**Response (SSE events):**
```
data: {"content":"The architecture is..."}
data: {"done":true,"tokens":843}
```

### GET /v2/status — Health and status

```bash
curl http://localhost:8080/v2/status
```

**Response:**
```json
{
  "status": "healthy",
  "version": "4.3.0",
  "providers": {
    "ollama": {"enabled": true, "status": "ready"},
    "openai": {"enabled": true, "status": "ready"},
    "anthropic": {"enabled": false, "status": "disabled"},
    "google": {"enabled": false, "status": "disabled"}
  },
  "tools": {"count": 15, "enabled": 15},
  "memory": {
    "short_term": {"messages": 12, "tokens": 3420},
    "long_term": {"entries": 156, "fts_enabled": true}
  },
  "sessions": {"active": 3, "total": 47},
  "ollama_queue": {
    "depth": 2,
    "capacity": 32,
    "processing": false
  },
  "long_term_memory": {"backend": "sqlite", "db_path": "data/memory.db"}
}
```

### GET /v2/tools — List available tools

```bash
curl http://localhost:8080/v2/tools
```

**Response:**
```json
{
  "tools": [
    {"name": "web_search", "description": "Search the web"},
    {"name": "code_search", "description": "Search codebase"},
    ...
  ],
  "count": 15,
  "enabled": 15
}
```

### GET /v2/memory — Memory statistics

```bash
curl http://localhost:8080/v2/memory
```

**Response:**
```json
{
  "short_term": {"messages": 12, "tokens": 3420},
  "long_term": {"entries": 156, "fts_enabled": true}
}
```

### POST /v2/session/new — Create session

```bash
curl -X POST http://localhost:8080/v2/session/new
```

**Response:**
```json
{
  "session_id": "uuid-v4-session-id",
  "created_at": "2025-06-10T14:30:00Z"
}
```

### DELETE /v2/session/{id} — Delete session

```bash
curl -X DELETE http://localhost:8080/v2/session/uuid-v4-session-id
```

**Response:**
```json
{
  "session_id": "uuid-v4-session-id",
  "deleted": true
}
```

### POST /v2/tool — Execute tool directly

```bash
curl -X POST http://localhost:8080/v2/tool \
  -H "Content-Type: application/json" \
  -d '{
    "tool_name": "web_search",
    "parameters": {
      "query": "Go best practices 2025"
    }
  }'
```

**Response:**
```json
{
  "result": "...search results...",
  "error": null
}
```

### WebSocket — /v2/ws

Connect for real-time bidirectional communication:

```javascript
const ws = new WebSocket('ws://localhost:8080/v2/ws');

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  // msg.type: "chat" | "tool" | "status" | "error" | "pong"
};

ws.send(JSON.stringify({
  type: "chat",
  message: "hello",
  session_id: "my-session"
}));
```

---

## 🔌 gRPC API

The gRPC service is available on port `50051` (default).

### Proto Definition

```protobuf
syntax = "proto3";
package agent;

service AgentService {
  rpc Chat(ChatRequest) returns (ChatResponse);
  rpc StreamChat(ChatRequest) returns (stream ChatResponse);
  rpc Status(StatusRequest) returns (StatusResponse);
}

message ChatRequest {
  string message = 1;
  string provider = 2;
  string session_id = 3;
}

message ChatResponse {
  string message = 1;
  string model = 2;
  int32 tier = 3;
  int64 tokens = 4;
  string session_id = 5;
}

message StatusRequest {}

message StatusResponse {
  string status = 1;
  string version = 2;
  int32 providers = 3;
  int32 tools = 4;
}
```

### gRPC Client Example (Go)

```go
package main

import (
    "context"
    "log"
    "time"

    pb "github.com/nicholas/ai-agent/grpc/proto"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    conn, err := grpc.NewClient("localhost:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewAgentServiceClient(conn)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Simple request
    resp, err := client.Chat(ctx, &pb.ChatRequest{
        Message:    "analyze the auth service for security issues",
        Provider:  "openai",
        SessionId: "my-session-1",
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Response: %s (Tier: %d, Tokens: %d)",
        resp.Message, resp.Tier, resp.Tokens)
}

func streamExample() {
    conn, _ := grpc.NewClient("localhost:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()))
    defer conn.Close()

    client := pb.NewAgentServiceClient(conn)
    ctx, _ := context.WithTimeout(context.Background(), 5*time.Minute)

    stream, err := client.StreamChat(ctx, &pb.ChatRequest{
        Message:   "explain the codebase architecture",
        Provider: "ollama",
    })
    if err != nil {
        log.Fatal(err)
    }

    for {
        resp, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Fatal(err)
        }
        fmt.Print(resp.Message)
    }
}
```

---

## ⚡ Ollama Request Queue

For single-GPU Ollama deployments, Kyoci Agent includes a **built-in request queue**:

- **Serial execution** — One LLM request at a time (no GPU contention)
- **Async back-pressure** — Accepts up to 32 pending requests, blocks beyond
- **Fair FIFO ordering** — First-in-first-out processing
- **Metrics** — Queue depth and status exposed at `/v2/status`

**Why it matters:** Without queuing, concurrent Ollama requests can cause OOM crashes or extreme latency. The queue ensures predictable, stable performance on resource-constrained hardware.

---

## 🛡️ Security Middleware

Kyoci Agent includes optional security features (disabled by default, enable in config):

### API Key Authentication

```yaml
security:
  auth:
    enabled: true
    api_keys:
      - "sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8"  # "secret"
```

Uses SHA256 prefix matching for secure, constant-time verification.

### Rate Limiting

```yaml
security:
  rate_limit:
    enabled: true
    requests_per_minute: 60
    window_size_seconds: 60
```

Sliding window token bucket algorithm — accurate and memory-efficient.

### Input Sanitization

Path traversal, command injection, and template injection are automatically sanitized. All user inputs are validated before execution.

---

## 💾 WAL Durable Execution

The DAGExecutor now supports **Write-Ahead Logging** for crash recovery:

- **Automatic checkpointing** — After each DAG step, state is persisted
- **Crash recovery** — On restart, incomplete DAGs are resumed from checkpoint
- **Idempotent steps** — Safe to retry failed operations
- **Storage** — SQLite-backed WAL (`data/dag_wal.db`)

**Use case:** Long-running multi-step plans (self-improvement pipeline, code refactoring) can safely survive server restarts.

---

## 📊 Comparison

| Feature | PicoClaw | Hermes Agent | **Kyoci Agent** |
|---------|----------|--------------|-----------------|
| Language | Go | Python | **Go** |
| Binary Size | ~10MB | ~100MB+ | **25MB** |
| MCP Client | ✅ | ❌ | ✅ |
| AST + LSP Code Intel | ❌ | ❌ | ✅ |
| Tiered Routing (0→1→2) | ❌ | ❌ | ✅ |
| Sub-Agent Delegation | ❌ | ✅ | ✅ |
| FTS5 Memory | ❌ | ✅ | ✅ |
| Multi-Backend Sandbox | ❌ | ✅ (5) | ✅ (4) |
| WAL Durable Execution | ❌ | ❌ | ✅ |
| Distributed Tracing | ❌ | ❌ | ✅ |
| Zero-Token Skills | ❌ | ❌ | ✅ (8) |
| Self-Improvement | ❌ | ✅ | ✅ |
| Prompt Caching | ❌ | ❌ | ✅ |
| Structured Logging | ❌ | ❌ | ✅ (slog) |
| Graceful Shutdown | ❌ | ❌ | ✅ |

---

## 🛠️ Zero-AI Skills (Instant, Free)

| Skill | Example | Latency |
|-------|---------|---------|
| Math | `hitung 25*37` → `925` | < 1ms |
| Time | `jam berapa` → `08:59:09` | < 1ms |
| Hash | `hash sha256 hello` → `2cf2...` | < 1ms |
| UUID | `generate uuid` → `550e...` | < 1ms |
| Base64 | `base64 encode hello` → `aGVs...` | < 1ms |
| Convert | `convert 37 celsius to fahrenheit` → `98.6°F` | < 1ms |
| Encode | `encode url hello world` → `hello%20world` | < 1ms |
| Weather | `weather Jakarta` → `32°C, Partly Cloudy` | < 1ms |

---

## 📦 Dependencies (5 direct)

```
github.com/google/uuid          v1.6.0     — UUID generation
google.golang.org/grpc          v1.81.1    — gRPC framework
google.golang.org/protobuf      v1.36.11   — Protocol buffers
gopkg.in/yaml.v3                v3.0.1     — YAML config
modernc.org/sqlite              v1.52.0    — Pure Go SQLite (FTS5)
```

Everything else — **pure Go stdlib**. Zero bloat.

---

## 📝 License

MIT License — use it, fork it, ship it.

---

<p align="center">
  <strong>Built with ⚡ by Kyoci</strong>
</p>
