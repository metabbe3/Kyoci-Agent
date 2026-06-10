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

## 🔌 Using Protobuf / gRPC

### Proto Definition

The gRPC API is defined in `proto/agent.proto`:

```protobuf
syntax = "proto3";
package agent;

// Kyoci Agent gRPC Service
service AgentService {
  // Process a single request
  rpc Process(AgentRequest) returns (AgentResponse);
  
  // Stream processing with real-time updates
  rpc StreamProcess(AgentRequest) returns (stream AgentResponse);
  
  // Health check
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

### Regenerate Protobuf Files

If you modify `proto/agent.proto`, regenerate the Go files:

```bash
# Install protoc plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       proto/agent.proto
```

### gRPC Client Example

```go
package main

import (
    "context"
    "log"
    "time"

    pb "github.com/nicholas/ai-agent/proto"
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
    resp, err := client.Process(ctx, &pb.AgentRequest{
        Prompt:     "analyze the auth service for security issues",
        Provider:   "openai",
        Model:      "gpt-4o",
        MaxTokens:  4096,
        SessionId:  "my-session-1",
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Content)
}
```

### Streaming Example

```go
stream, err := client.StreamProcess(ctx, &pb.AgentRequest{
    Prompt: "explain the codebase architecture",
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
    fmt.Print(resp.Content)
}
```

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

## 🔧 REST API

### Chat

```bash
curl -X POST http://localhost:8080/api/v2/chat \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-key" \
  -d '{
    "prompt": "analyze this codebase for security issues",
    "provider": "openai",
    "session_id": "optional-session-id"
  }'
```

### Streaming (SSE)

```bash
curl -N http://localhost:8080/api/v2/chat/stream \
  -H "Content-Type: application/json" \
  -d '{"prompt": "explain the architecture", "provider": "ollama"}'
```

### Health

```bash
curl http://localhost:8080/api/v2/health
```

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
