#!/usr/bin/env bash
# run_level_3.sh — orchestrate the Kyoci-Agent L3 (Memory + MCP + Compaction) benchmark.
#
# Unlike L1/L2 (which hit the running main agent on :8080), L3 manages a
# DEDICATED bench agent on :18080 with an isolated config (config/bench.yaml).
# This keeps Session A and Session B on the same process (so L3 SQLite memory
# persists between them) while isolating benchmark writes from production data.
#
# Flow:
#   preconditions → build mcp-mock → build bench agent → fresh bench DB
#   → start bench agent (:18080) → wait health → clean app_test_env
#   → POST Session A (teach preferences) → sleep (compaction)
#   → POST Session B (recall + execute via MCP) → kill bench agent → grade
#
# Idempotent: rm bench DB + app_test_env on each run.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

BENCH_PORT="${BENCH_PORT:-18080}"
BENCH_HEALTH="http://localhost:${BENCH_PORT}/health"
BENCH_WEBHOOK="http://localhost:${BENCH_PORT}/api/v1/webhook"
OLLAMA_TAGS_URL="${OLLAMA_TAGS_URL:-http://192.168.2.1:11434/api/tags}"
MODEL_NAME="${MODEL_NAME:-gemma4:12b}"
BENCH_DB="${BENCH_DB:-/tmp/kyoci-l3-bench.db}"
BENCH_LOG="${BENCH_LOG:-/tmp/kyoci-l3-bench.log}"
ENV_DIR="app_test_env"
RESP_A="${RESP_A:-benchmarks/last_response_level_3_a.json}"
RESP_B="${RESP_B:-benchmarks/last_response_level_3_b.json}"
TIMEOUT_A="${TIMEOUT_A:-120}"
TIMEOUT_B="${TIMEOUT_B:-360}"

echo ">> Kyoci-Agent Benchmark L3 — Memory, Auto-Compaction & MCP Integration"
echo ">> repo: $REPO_ROOT"
echo

# ── Preconditions ────────────────────────────────────────────────────────────
echo ">> Checking preconditions..."
if ! curl -sf "$OLLAMA_TAGS_URL" 2>/dev/null | grep -q "$MODEL_NAME"; then
  echo "!! FAIL: model '$MODEL_NAME' not available at $OLLAMA_TAGS_URL"
  exit 1
fi
echo "   model $MODEL_NAME available"
if command -v sqlite3 >/dev/null 2>&1; then
  echo "   sqlite3 CLI available"
else
  echo "!! WARNING: sqlite3 CLI not found — Metric 3 (Auto-Compaction) will be skipped"
fi
echo

# ── Step 1: Build binaries ───────────────────────────────────────────────────
echo ">> Step 1: building mcp-mock + bench agent"
mkdir -p bin
go build -o bin/mcp-mock ./cmd/mcp-mock
echo "   built bin/mcp-mock"
go build -o bin/kyoci-bench ./cmd/server
echo "   built bin/kyoci-bench"
echo

# ── Step 2: Fresh bench DB + clean env ───────────────────────────────────────
echo ">> Step 2: preparing isolated environment"
rm -f "$BENCH_DB"
echo "   removed $BENCH_DB"
rm -rf "$ENV_DIR"
mkdir -p "$ENV_DIR"
echo "   cleaned $ENV_DIR"
echo

# ── Step 3: Start bench agent ────────────────────────────────────────────────
echo ">> Step 3: starting bench agent on :${BENCH_PORT}"
BENCH_PID=""
cleanup() {
  if [[ -n "$BENCH_PID" ]] && kill -0 "$BENCH_PID" 2>/dev/null; then
    kill "$BENCH_PID" 2>/dev/null || true
    wait "$BENCH_PID" 2>/dev/null || true
    echo "   bench agent stopped (pid $BENCH_PID)"
  fi
}
trap cleanup EXIT

./bin/kyoci-bench -config config/bench.yaml > "$BENCH_LOG" 2>&1 &
BENCH_PID=$!
echo "   bench agent pid: $BENCH_PID"

# Wait for health (up to 30s)
healthy=0
for i in $(seq 1 30); do
  if curl -sf "$BENCH_HEALTH" >/dev/null 2>&1; then
    healthy=1
    break
  fi
  sleep 1
done
if [[ "$healthy" -ne 1 ]]; then
  echo "!! FAIL: bench agent did not become healthy within 30s"
  echo "   last 20 log lines:"
  tail -20 "$BENCH_LOG" || true
  exit 1
fi
echo "   bench agent healthy at $BENCH_HEALTH"
echo

# ── Step 4: Session A — teach preferences ────────────────────────────────────
echo ">> Step 4: Session A — teaching preferences (timeout ${TIMEOUT_A}s)"
LOG_START=$(wc -l < "$BENCH_LOG" | awk '{print $1}')
LOG_START=$((LOG_START + 1))
echo "   log start line: $LOG_START"

start_ts=$(date +%s)
HTTP_CODE_A=$(curl -s -o "$RESP_A" -w "%{http_code}" \
  -X POST "$BENCH_WEBHOOK" \
  -H "Content-Type: application/json" \
  -d @benchmarks/task_level_3_session_a.json \
  --max-time "$TIMEOUT_A") || true
end_ts=$(date +%s)
elapsed_a=$((end_ts - start_ts))
echo "   Session A: HTTP $HTTP_CODE_A in ${elapsed_a}s"

# Give the async recorder goroutine + compaction time to settle.
sleep 3
echo

# ── Step 5: Session B — recall + execute via MCP ─────────────────────────────
echo ">> Step 5: Session B — recall + MCP tool execution (timeout ${TIMEOUT_B}s)"
start_ts=$(date +%s)
HTTP_CODE_B=$(curl -s -o "$RESP_B" -w "%{http_code}" \
  -X POST "$BENCH_WEBHOOK" \
  -H "Content-Type: application/json" \
  -d @benchmarks/task_level_3_session_b.json \
  --max-time "$TIMEOUT_B") || true
end_ts=$(date +%s)
elapsed_b=$((end_ts - start_ts))
echo "   Session B: HTTP $HTTP_CODE_B in ${elapsed_b}s"
echo

# ── Step 6: Kill bench agent + grade ─────────────────────────────────────────
echo ">> Step 6: stopping bench agent + grading"
cleanup
trap - EXIT
echo

bash benchmarks/grade_level_3.sh "$LOG_START"
