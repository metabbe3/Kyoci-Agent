# Kyoci Agent v5

A Go-based multi-agent platform built for **small local models** (8B–14B). Six specialist roles, a 151-entry built-in catalog (125 skills + 26 tools), an L3 SQLite memory layer, and a gRPC-streamed Human-In-The-Loop fallback — all orchestrated by a Go-driven pipeline that gives each LLM call exactly one job.

```
╔═══════════════════════════════════════╗
║       Kyoci Agent v5 — MVP            ║
║       Plug & Play AI Agent Platform    ║
╚═══════════════════════════════════════╝

HTTP API:    http://localhost:8080
HITL gRPC:   localhost:50052   (max_retries=2)
Roles:       6   (developer, sre, qa, pm, frontend, generalist)
Skills:      125 (zero-AI, deterministic — skip the LLM call)
Tools:       26  (file/shell/code/web + 4 intelligence hooks)
Providers:   20  (Ollama, LM Studio, OpenAI, Anthropic, Gemini, ...)
```

---

## Quickstart

**One-click (macOS):**

```bash
./start.command
```

Checks deps, starts the Go server on `:8080`, builds and launches the React dashboard.

**Manual:**

```bash
# Backend
go run ./cmd/server -config config/default.yaml

# Frontend (separate terminal)
cd web && npm install && npm run dev

# Operator terminal (optional — for HITL fallback)
go run ./cmd/hitlctl --interactive
```

**Smoke check:**

```bash
curl http://localhost:8080/health
# {"service":"kyoci-agent","status":"ok","version":"5.0.0"}

curl -X POST http://localhost:8080/api/v1/webhook \
  -H "Content-Type: application/json" \
  -d '{"task":"sha256 of hello","timeout":60}'
```

---

## How it works

Each task flows through a 4-phase Go-driven pipeline. Per-call jobs are small enough that a 14B model handles them reliably.

```
           ┌──────────┐   ┌────────────┐   ┌──────────┐   ┌─────────────┐
  task ──► │ Planner  │ ─►│ Dispatcher │ ─►│ Workers  │ ─►│ Synthesizer │ ─► answer
           │ (1 LLM)  │   │ (pure Go)  │   │ (N LLMs) │   │ (1 LLM)     │
           └──────────┘   └────────────┘   └──────────┘   └─────────────┘
               │                              │ ▲
               │                              │ │ skill fast-path: when step
               │                              │ │ carries tool_hint="skill",
               │                              │ │ skip the worker LLM call.
               ▼                              ▼
           ClassifyRole()              a.skills.Match(task)
           (pure-Go confidence         (125 zero-AI deterministic
            score → role)               transformations)
```

When a task carries a `VERIFY:` directive, the orchestrator wraps the pipeline in an outer retry loop with HITL fallback (see [HITL](#hitl-fallback)).

---

## Roles (6)

| Role | Codename | Scope |
|---|---|---|
| **generalist** | Sage | Research, explanation, multi-domain. Classifier fallback. |
| **developer**  | Forge | Code in Go/Rust/Python/etc. — file write + terminal. |
| **frontend**   | Lumen | React/TS/CSS — `file write` enforces actual writes. |
| **qa**         | Sift  | Test authoring + `security_scan`. |
| **sre**        | Beacon| Ops, deploy, k8s, observability. |
| **pm**         | Aria  | Planning, web research, written comms. |

Each specialist has a `DELEGATION` block in its system prompt and the `delegation` tool — they hand off subtasks to each other in parallel (max 3 concurrent, 180s each).

---

## Catalog

### Skills (125, zero-AI)

Match()'d by the registry and executed **without an LLM call** — instant and free.

| Category | Count | Skills |
|---|---:|---|
| **encoding** | 12 | base64, base32, url, html, hex, unicode (encode + decode) |
| **hashing** | 13 | md5, sha1, sha256, sha512, sha3_256, crc32, crc64, hmac_sha256, hmac_sha512, bcrypt_hash, bcrypt_verify, aes_encrypt, aes_decrypt |
| **security** | 4 | password_strength, secret_redact, hash_identify, cve_parse |
| **datafmt** | 12 | yaml↔json, toml↔json, csv↔json, xml↔json, env↔json, json_minify, json_pretty |
| **text** | 15 | slugify, case_convert, levenshtein, char/word/line/byte_count, truncate, pad, reverse, sort_lines, dedupe_lines, indent, dedent, regex_replace |
| **generators** | 10 | uuid_v4, uuid_v7, nanoid, guid, random_int, random_string, random_bytes, nonce, fake_name, fake_email |
| **net** | 9 | ip_validate, ip_info, mac_lookup, port_check, url_parse, url_build, cidr_validate, cidr_merge, dns_lookup |
| **color** | 8 | hex_to_rgb, rgb_to_hex, hex_to_hsl, hsl_to_hex, contrast_ratio, color_blend, palette_analogous, palette_complementary |
| **math** | 12 | stats, gcd, lcm, is_prime, prime_factors, factorial, base_convert, round_sig, units_convert, currency_format, percentage, ratio_simplify |
| **time** | 6 | now, time_parse, time_format, time_diff, cron_next, epoch_convert |
| **markdown** | 4 | markdown_outline, markdown_toc, markdown_strip, markdown_link_extract |
| **legacy** | 20 | math (evaluator), time, hash, uuid, encode, convert, color (all-reps), regex, jsonfmt, sqlfmt, diff, jwt, qr, password, charset, cron, subnet, lorem, markdown, emojinfo |

### Tools (26, LLM-invokable)

| Tool | What it does |
|---|---|
| `terminal` | Shell command execution (with dangerous-command blocklist) |
| `file` | Read/write/edit/list/search files |
| `http_client` | HTTP requests |
| `web_search` | Web search |
| `calculator` | Math evaluation |
| `browser` | Headless browser automation |
| `docs` | Document processing |
| `todo` | Task list |
| `process` | Run/kill subprocesses |
| `uploaded_file` | Parse uploads |
| `excel` | .xlsx reader |
| `skill` | Save/load reusable procedures |
| **`patch`** | Hermes-style fuzzy find/replace with syntax verify + auto-revert |
| **`grep`** | Ripgrep-backed content search (Go-regex fallback) |
| **`glob`** | `**`-aware file pattern matcher |
| **`git`** | Read-only git operations (status/diff/log/branch/show/blame) |
| **`lsp`** | Code intelligence via gopls / typescript-language-server |
| **`web_fetch`** | URL → cleaned markdown (strips scripts/nav) |
| **`secret_scan`** | Scan a tree for AWS/GitHub/Stripe/JWT/private-key patterns |
| **`notes`** | Agent scratchpad (separate from L3 memory) |
| **`format`** | gofmt / prettier / rustfmt / black wrapper |
| **`codesearch`** | grep + enclosing function (saves a follow-up read) |
| `memory_recall` | L3 SQLite search (intelligence hook) |
| `remember` | Persist user fact/preference (intelligence hook) |
| `security_scan` | OWASP-style vulnerability scan (intelligence hook) |
| `delegation` | Spawn specialist sub-agents in parallel (intelligence hook) |

---

## HITL fallback

When an agent exhausts its retry budget on a `VERIFY:`-bearing task, it pauses and asks a human operator for a hint over gRPC. The hint is injected into the next pipeline pass. On success, a permanent "lesson learned" is recorded in L3 SQLite so future sessions benefit.

```
task with "VERIFY: go test ./..."
   │
   ▼
   attempt 1 → run agent → run verify → FAIL
   attempt 2 → run agent → run verify → FAIL   (max_retries=2)
   ─────────── HITL emits HelpRequest via gRPC ───────────
   operator (cmd/hitlctl) submits hint
   ────────────────────────────────────────────────────────
   attempt 3 → run agent WITH HINT → run verify → PASS
   │
   ▼
   ReflectionEngine.RecordLesson → L3 SQLite
```

**Try it:**

```bash
# T1: server (HTTP :8080 + HITL gRPC :50052)
go run ./cmd/server

# T2: operator terminal
go run ./cmd/hitlctl --interactive

# T3: trigger the benchmark
bash benchmarks/run_level_4.sh
bash benchmarks/grade_level_4.sh   # → LEVEL 4: X/4 PASS
```

For hands-off CI, replace T2 with `hitlctl --auto --hint-file=benchmarks/hint_level_4.txt`.

---

## Configuration

`config/default.yaml` — all sections env-overridable.

```yaml
server:    { grpc_port: 50051, rest_port: 8080 }
hitl:      { enabled: true, port: 50052, request_timeout: 300 }
agent:
  orchestration:
    enabled: true          # 4-phase pipeline (default)
    max_retries: 2         # pre-HITL retries on VERIFY failure
    max_steps: 6
    max_parallel: 3
    worker_max_iterations: 8
    worker_max_tool_calls: 8
providers:
  ollama:   { enabled: false, base_url: "http://localhost:11434/v1" }
  lmstudio: { enabled: true,  base_url: "http://127.0.0.1:1234/v1" }   # default
  openai:   { enabled: false, base_url: "https://api.openai.com/v1" }
  # ... + 17 more (anthropic, gemini, groq, mistral, deepseek, ...)
```

Env overrides follow `KYOCI_<SECTION>_<KEY>` conventions (e.g. `KYOCI_DB_PATH=/tmp/bench.db`).

---

## Benchmark suite

`benchmarks/` — graded end-to-end runs against a live model.

| Level | What it tests | Scripts |
|---|---|---|
| L1 | Single-task webhook | `run_benchmark.sh`, `grade.sh` |
| L2 | Multi-step orchestration | `run_level_2.sh`, `grade_level_2.sh` |
| L3 | Memory + MCP + auto-compaction | `run_level_3.sh`, `grade_level_3.sh` |
| L4 | Self-healing + HITL + self-learning | `run_level_4.sh`, `grade_level_4.sh` |

`bash benchmarks/run_all.sh` to execute L1–L3 sequentially.

---

## Project structure

```
cmd/
  server/      # HTTP :8080 + HITL gRPC :50052 entry point
  hitlctl/     # operator CLI for HelpRequest stream
  mcp-mock/    # test helper for L3 benchmark
internal/
  agent/           # 4-phase orchestrated pipeline + ReAct loop
  config/          # YAML + env-override loader
  dashboard/       # embedded SPA + REST API for the dashboard
  gateway/         # Telegram bot gateway
  hardware/        # system telemetry panel
  hitl/            # gRPC Hub + Server + HITLHook
  llm/             # providers, router, OpenAI-compatible client
  mcp/             # MCP server manager
  memory/          # L1/L2/L3 SQLite + Reflection + Profile + Experience
  orchestrator/    # Execute(), classifier, HITL retry loop
  recommend/       # model-recommendation panel
  role/            # 6 roles (developer, sre, qa, pm, frontend, generalist)
  skill/           # 125-skill registry
  tool/            # 26-tool registry + built-in tools
  tracing/         # lightweight span tracer
pkg/               # public interfaces (Tool, Skill, Role, Memory, Provider)
proto/             # .proto definitions (hitl.proto is compiled)
web/               # React 18 + Vite + Tailwind dashboard
benchmarks/        # L1–L4 graded benchmark suite
config/            # default.yaml (+ bench configs)
```

---

## Tech stack

**Backend:** Go 1.25 · `modernc.org/sqlite` (pure-Go SQLite) · `google.golang.org/grpc` · `gopkg.in/yaml.v3` · `github.com/pelletier/go-toml/v2` · `golang.org/x/crypto` (bcrypt, pbkdf2, sha3)

**Frontend:** React 18 · TypeScript · Vite · Tailwind · Radix UI · Motion · Recharts · Lenis (smooth scroll) · react-markdown

**Tested providers:** Ollama, LM Studio (default). Wire-compatible with OpenAI, Anthropic, Gemini, Groq, Mistral, DeepSeek, Together, Fireworks, xAI, Cohere, Perplexity, OpenRouter, LiteLLM, Cloudflare, NVIDIA NIM, Moonshot, Qwen, ZAI.

---

## Verification

```bash
go build ./...     # all packages compile
go vet ./...       # clean
go test ./...      # all 15 packages pass
```

Server startup log:

```
orchestrator initialized successfully — providers:1, roles:6, tools:26, skills:125
HITL gRPC server listening :50052  (max_retries=2)
HTTP server listening :8080
```
