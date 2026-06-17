# Kyoci-Agent Benchmark Suite

A set of reusable end-to-end benchmarks that grade the Kyoci-Agent on
realistic developer tasks. Each level exercises a different aspect of the
Orchestrator-Worker pipeline. Re-run the whole suite after any change to the
orchestrator, worker guard, LLM normalizer, or model.

## Levels

| Level | Script | Task | Tests |
|-------|--------|------|-------|
| **L1 — Scattered Dependency Bug** | `run_benchmark.sh` | Find a hardcoded MongoDB credential, refactor to `process.env.DATABASE_URL`, create `.env.example`. | Planner decomposition (≥2 steps), worker tool use, code accuracy, file creation. |
| **L2 — Spaghetti Refactor** | `run_level_2.sh` | Split a monolithic `server.js` into `routes/` + `controllers/` + `services/`, clean entrypoint, README. | Planner decomposition (≥4 steps), directory creation, clean entrypoint, time limit (<300s), documentation. |
| **L3 — Memory + MCP + Compaction** | `run_level_3.sh` | Session A teaches coding preferences; Session B recalls them via L3 memory, fetches a user schema via an MCP tool, and generates a matching `main.go`. | L3 memory retrieval, MCP tool execution, auto-compaction persistence, time limit (<300s), zero-hallucination. |

**L3 differs from L1/L2:** L1 and L2 hit the main agent running on `:8080`.
L3 manages its **own dedicated bench agent** on `:18080` (built from
`config/bench.yaml`) so that Session A and Session B share the same process and
the same L3 SQLite DB, while keeping benchmark writes isolated from production
data.

## Quick start

```bash
# Run one level:
bash benchmarks/run_benchmark.sh        # L1 (uses main agent on :8080)
bash benchmarks/run_level_2.sh          # L2 (uses main agent on :8080)
bash benchmarks/run_level_3.sh          # L3 (manages own bench agent on :18080)

# Run the whole suite:
bash benchmarks/run_all.sh
```

Preconditions:
- **L1/L2:** the main agent must be reachable at `http://localhost:8080/health`
  and the configured model (default `gemma4:12b`) must be loaded in Ollama.
- **L3:** only the Ollama model precondition is checked — `run_level_3.sh`
  builds and starts its own bench agent. The `sqlite3` CLI is optional (Metric 3
  is skipped if absent).

## Report Cards

Each script prints a Report Card to stdout AND writes it to a file:

| Output | Path |
|--------|------|
| L1 standalone Report Card | `benchmarks/last_report.txt` |
| L2 standalone Report Card | `benchmarks/last_report_level_2.txt` |
| L3 standalone Report Card | `benchmarks/last_report_level_3.txt` |
| L1 suite Report Card      | `benchmarks/last_report_level_1.txt` |
| L2 suite Report Card      | `benchmarks/last_report_level_2.txt` |
| L3 suite Report Card      | `benchmarks/last_report_level_3.txt` |
| L1 webhook response       | `benchmarks/last_response.json` |
| L2 webhook response       | `benchmarks/last_response_level_2.json` |
| L3 Session A response     | `benchmarks/last_response_level_3_a.json` |
| L3 Session B response     | `benchmarks/last_response_level_3_b.json` |
| Suite dispatcher logs     | `benchmarks/last_run_level_{1,2,3}.log` |

`run_all.sh` parses the `Overall: N/M PASS` line from each level and prints
a combined summary table at the end.

## Files

```
benchmarks/
├── run_benchmark.sh                 # L1 orchestrator
├── grade.sh                         # L1 evaluator (4 metrics)
├── task.json                        # L1 webhook payload
├── run_level_2.sh                   # L2 orchestrator
├── grade_level_2.sh                 # L2 evaluator (5 metrics)
├── task_level_2.json                # L2 webhook payload
├── run_level_3.sh                   # L3 orchestrator (bench agent lifecycle + 2 sessions)
├── grade_level_3.sh                 # L3 evaluator (5 metrics)
├── task_level_3_session_a.json      # L3 Session A webhook payload (teach preferences)
├── task_level_3_session_b.json      # L3 Session B webhook payload (recall + execute)
├── run_all.sh                       # Runs L1 + L2 + L3 sequentially, prints combined summary
└── README.md                        # (this file)
agent_test_env/                      # L1 runtime env (gitignored, recreated each run)
app_test_env/                        # L2 + L3 runtime env (gitignored, recreated each run)
bin/                                 # L3-built binaries: mcp-mock, kyoci-bench (gitignored)
```

L3 also depends on two files outside `benchmarks/`:
- `config/bench.yaml` — bench-agent config (Telegram off, isolated DB, MCP mock wired, port 18080)
- `cmd/mcp-mock/main.go` — the stdio JSON-RPC mock MCP server exposing `fetch_user_schema`

## Re-running after architecture changes

All scripts are idempotent: they `rm -rf` the runtime env before each run and
scope log greps to the current run's offset. Just re-run after any change.

## Environment overrides

All scripts honor these env vars (defaults shown):

| Variable | Default | Purpose |
|----------|---------|---------|
| `WEBHOOK_URL` | `http://localhost:8080/api/v1/webhook` | Agent endpoint (L1/L2). |
| `HEALTH_URL` | `http://localhost:8080/health` | Agent health check (L1/L2). |
| `OLLAMA_TAGS_URL` | `http://192.168.2.1:11434/api/tags` | Ollama model list. |
| `MODEL_NAME` | `gemma4:12b` | Required model name. |
| `KYOCI_LOG` | `/tmp/kyoci-agent.log` | Agent log path (L1/L2). |
| `TIMEOUT_SECS` | `360` | Webhook request timeout (L1/L2). |
| `BENCH_PORT` | `18080` | Bench agent port (L3 only). |
| `BENCH_DB` | `/tmp/kyoci-l3-bench.db` | Bench SQLite DB path (L3 only). |
| `BENCH_LOG` | `/tmp/kyoci-l3-bench.log` | Bench agent log path (L3 only). |
| `TIMEOUT_A` | `120` | Session A webhook timeout (L3 only). |
| `TIMEOUT_B` | `360` | Session B webhook timeout (L3 only). |
| `REPORT_FILE` | (per-script default) | Where the Report Card is written. |
| `RESP_FILE` | (per-script default) | Where the webhook response is written. |
| `ENV_DIR` | (per-script default) | Runtime test env directory. |

## Adding a new level

1. Add `task_level_N.json` with the webhook payload.
2. Add `setup_level_N` logic (write the deliberately-broken starter files).
3. Add `grade_level_N.sh` with the rubric; honor `REPORT_FILE` / `RESP_FILE` /
   `ENV_DIR` / `LOG_START` like the existing grade scripts.
4. Add a stanza to `run_all.sh` that calls the new level with a level-specific
   `REPORT_FILE` and parses its `Overall:` line into the combined summary.
