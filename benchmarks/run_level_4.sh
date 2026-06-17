#!/usr/bin/env bash
# run_level_4.sh — orchestrate the Kyoci-Agent L4 (Self-Healing + HITL + Self-Learning) benchmark.
#
# Flow:
#   preconditions → build server + hitlctl → reset buggy calculator.go
#   → start server (:8080 HTTP + :50052 HITL gRPC) → start hitlctl --auto
#   → POST task (with VERIFY: directive) → wait → kill → grade
#
# The agent must:
#   1. Run go test on app_test_env_l4, parse the failure
#   2. Try to fix calculator.go (attempts 1-2 will fail)
#   3. After 2 retries, the orchestrator pauses and emits a HelpRequest
#   4. hitlctl --auto submits the pre-baked hint
#   5. Agent applies the hint, fixes the bug, test passes
#   6. Orchestrator records a "lesson learned" in L3 SQLite
#
# Idempotent: rm bench DB + reset buggy calculator.go on each run.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

PORT="${PORT:-8080}"
HITL_PORT="${HITL_PORT:-50052}"
HEALTH="http://localhost:${PORT}/health"
WEBHOOK="http://localhost:${PORT}/api/v1/webhook"
OLLAMA_TAGS_URL="${OLLAMA_TAGS_URL:-http://192.168.2.1:11434/api/tags}"
MODEL_NAME="${MODEL_NAME:-gemma4:12b}"
BENCH_DB="${BENCH_DB:-/tmp/kyoci-l4-bench.db}"
BENCH_LOG="${BENCH_LOG:-/tmp/kyoci-l4-bench.log}"
HITL_LOG="${HITL_LOG:-/tmp/kyoci-l4-hitlctl.log}"
RESP_FILE="${RESP_FILE:-benchmarks/last_response_level_4.json}"
ENV_DIR="benchmarks/app_test_env_l4"
CALC_FILE="${ENV_DIR}/calculator.go"
HINT_FILE="${HINT_FILE:-benchmarks/hint_level_4.txt}"
TIMEOUT="${TIMEOUT:-360}"
CONFIG_FILE="${CONFIG_FILE:-config/default.yaml}"

echo ">> Kyoci-Agent Benchmark L4 — Self-Healing + HITL + Self-Learning"
echo ">> repo: $REPO_ROOT"
echo

# ── Preconditions ────────────────────────────────────────────────────────────
echo ">> Checking preconditions..."
if ! curl -sf --max-time 3 "$OLLAMA_TAGS_URL" 2>/dev/null | grep -q "$MODEL_NAME"; then
  echo "   (note: '$MODEL_NAME' not confirmed at $OLLAMA_TAGS_URL — using lmstudio fallback)"
else
  echo "   model $MODEL_NAME available at $OLLAMA_TAGS_URL"
fi
# Confirm at least one provider is reachable (lmstudio default).
LMSTUDIO_URL="${LMSTUDIO_URL:-http://127.0.0.1:1234/v1/models}"
if ! curl -sf --max-time 3 "$LMSTUDIO_URL" >/dev/null 2>&1; then
  echo "!! FAIL: no LLM provider reachable (checked $LMSTUDIO_URL)"
  exit 1
fi
echo "   lmstudio reachable at $LMSTUDIO_URL"
if command -v sqlite3 >/dev/null 2>&1; then
  echo "   sqlite3 CLI available"
else
  echo "!! FAIL: sqlite3 CLI required for grading"
  exit 1
fi
if [[ ! -f "$HINT_FILE" ]]; then
  echo "!! FAIL: hint file not found at $HINT_FILE"
  exit 1
fi
echo

# ── Step 1: Build binaries ───────────────────────────────────────────────────
echo ">> Step 1: building server + hitlctl"
mkdir -p bin
go build -o bin/kyoci-l4-server ./cmd/server
echo "   built bin/kyoci-l4-server"
go build -o bin/hitlctl ./cmd/hitlctl
echo "   built bin/hitlctl"
echo

# ── Step 2: Reset environment to known-buggy state ───────────────────────────
echo ">> Step 2: resetting environment to known-buggy state"
cat > "$CALC_FILE" <<'EOF'
package mathutils

// Add returns the sum of a and b.
// BUG: It currently multiplies instead of adding.
func Add(a, b int) int {
	return a * b
}
EOF
echo "   reset $CALC_FILE to buggy state (a * b)"

# Fresh bench DB so prior-run lessons don't pollute grading.
rm -f "$BENCH_DB"
echo "   removed $BENCH_DB"
echo

# ── Step 3: (config skipped) ─────────────────────────────────────────────────
# The default config has hitl.enabled=true, max_retries=2, and lmstudio on.
# We override only the DB path via KYOCI_DB_PATH env var so prior runs don't
# pollute grading. The default ports (:8080 HTTP, :50052 HITL gRPC) are used.
echo ">> Step 3: using config $CONFIG_FILE with DB override"
echo "   KYOCI_DB_PATH=$BENCH_DB"
echo

# ── Step 4: Start server (HTTP + HITL gRPC) ──────────────────────────────────
echo ">> Step 4: starting server (HTTP :$PORT + HITL gRPC :$HITL_PORT)"
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    echo "   server stopped (pid $SERVER_PID)"
  fi
  if [[ -n "${HITLCTL_PID:-}" ]] && kill -0 "$HITLCTL_PID" 2>/dev/null; then
    kill "$HITLCTL_PID" 2>/dev/null || true
    wait "$HITLCTL_PID" 2>/dev/null || true
    echo "   hitlctl stopped (pid $HITLCTL_PID)"
  fi
}
trap cleanup EXIT

KYOCI_DB_PATH="$BENCH_DB" \
  ./bin/kyoci-l4-server -config "$CONFIG_FILE" > "$BENCH_LOG" 2>&1 &
SERVER_PID=$!
echo "   server pid: $SERVER_PID"

healthy=0
for i in $(seq 1 30); do
  if curl -sf "$HEALTH" >/dev/null 2>&1; then
    healthy=1
    break
  fi
  sleep 1
done
if [[ "$healthy" -ne 1 ]]; then
  echo "!! FAIL: server did not become healthy within 30s"
  echo "   last 20 log lines:"
  tail -20 "$BENCH_LOG" || true
  exit 1
fi
echo "   server healthy at $HEALTH"
echo

# ── Step 5: Launch hitlctl in --auto mode ────────────────────────────────────
echo ">> Step 5: starting hitlctl --auto"
./bin/hitlctl \
  --addr="localhost:$HITL_PORT" \
  --auto \
  --hint-file="$HINT_FILE" \
  > "$HITL_LOG" 2>&1 &
HITLCTL_PID=$!
echo "   hitlctl pid: $HITLCTL_PID"

# Give hitlctl a moment to subscribe before we trigger the task.
sleep 2
echo

# ── Step 6: POST task ────────────────────────────────────────────────────────
echo ">> Step 6: POSTing task (timeout ${TIMEOUT}s)"
LOG_START=$(wc -l < "$BENCH_LOG" | awk '{print $1}')
LOG_START=$((LOG_START + 1))
echo "   log start line: $LOG_START"

start_ts=$(date +%s)
HTTP_CODE=$(curl -s -o "$RESP_FILE" -w "%{http_code}" \
  -X POST "$WEBHOOK" \
  -H "Content-Type: application/json" \
  -d @"benchmarks/task_level_4.json" \
  --max-time "$TIMEOUT") || true
end_ts=$(date +%s)
elapsed=$((end_ts - start_ts))
echo "   HTTP $HTTP_CODE in ${elapsed}s"
echo

# Give async lesson-recording a beat to flush.
sleep 2

# ── Step 7: Stop server + grade ──────────────────────────────────────────────
echo ">> Step 7: stopping server + grading"
cleanup
trap - EXIT
echo

bash benchmarks/grade_level_4.sh "$LOG_START"
