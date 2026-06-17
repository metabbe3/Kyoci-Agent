#!/usr/bin/env bash
# run_level_2.sh — orchestrate the Kyoci-Agent L2 (Spaghetti Refactor) benchmark.
#
# Flow:  preconditions → write spaghetti server.js → record log offset
#        → trigger agent → wait → grade → emit Report Card.
#
# Idempotent: cleans app_test_env/ before each run.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

WEBHOOK_URL="${WEBHOOK_URL:-http://localhost:8080/api/v1/webhook}"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/health}"
OLLAMA_TAGS_URL="${OLLAMA_TAGS_URL:-http://192.168.2.1:11434/api/tags}"
MODEL_NAME="${MODEL_NAME:-gemma4:12b}"
LOG_FILE="${KYOCI_LOG:-/tmp/kyoci-agent.log}"
TIMEOUT_SECS="${TIMEOUT_SECS:-360}"
ENV_DIR="app_test_env"
RESP_FILE="${RESP_FILE:-benchmarks/last_response_level_2.json}"

echo ">> Kyoci-Agent Benchmark L2 — Spaghetti Refactor"
echo ">> repo: $REPO_ROOT"
echo

# ── Preconditions ────────────────────────────────────────────────────────────
echo ">> Checking preconditions..."
if ! curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
  echo "!! FAIL: agent not reachable at $HEALTH_URL"
  exit 1
fi
echo "   agent healthy"
if ! curl -sf "$OLLAMA_TAGS_URL" 2>/dev/null | grep -q "$MODEL_NAME"; then
  echo "!! FAIL: model '$MODEL_NAME' not available at $OLLAMA_TAGS_URL"
  exit 1
fi
echo "   model $MODEL_NAME available"
if [[ ! -f "$LOG_FILE" ]]; then
  echo "!! FAIL: log file $LOG_FILE not found"
  exit 1
fi
echo "   log file readable"
echo

# ── Step 1: Setup — clean + recreate the spaghetti server.js ────────────────
echo ">> Step 1: recreating $ENV_DIR/server.js (monolith)"
rm -rf "$ENV_DIR"
mkdir -p "$ENV_DIR"

cat >"$ENV_DIR/server.js" <<'JS'
// server.js - Deliberately terrible spaghetti code
const express = require('express');
const app = express();
app.use(express.json());

let users = [{ id: 1, name: "Alice" }];

// Get users
app.get('/api/users', (req, res) => {
    res.status(200).json(users);
});

// Create user with inline validation and logic
app.post('/api/users', (req, res) => {
    if (!req.body.name || req.body.name.length < 3) {
        return res.status(400).json({ error: "Name too short" });
    }
    const newUser = { id: users.length + 1, name: req.body.name };
    users.push(newUser);
    res.status(201).json(newUser);
});

app.listen(3000, () => console.log('Server running on port 3000'));
JS

ORIG_LINES=$(wc -l < "$ENV_DIR/server.js" | awk '{print $1}')
echo "   wrote $ENV_DIR/server.js ($ORIG_LINES lines)"
echo "   monolith signals: data_array=$(grep -c 'let users =' "$ENV_DIR/server.js") routes_inline=$(grep -c "app\\.get\\|app\\.post" "$ENV_DIR/server.js")"
echo

# ── Step 2: Trigger ─────────────────────────────────────────────────────────
echo ">> Step 2: triggering agent via webhook (timeout ${TIMEOUT_SECS}s)"
LOG_START=$(wc -l < "$LOG_FILE" | awk '{print $1}')
LOG_START=$((LOG_START + 1))
echo "   log start line: $LOG_START"

start_ts=$(date +%s)
HTTP_CODE=$(curl -s -o "$RESP_FILE" -w "%{http_code}" \
  -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d @benchmarks/task_level_2.json \
  --max-time "$TIMEOUT_SECS") || true
end_ts=$(date +%s)
elapsed=$((end_ts - start_ts))

echo "   webhook returned HTTP $HTTP_CODE in ${elapsed}s"
if [[ "$HTTP_CODE" != "200" ]]; then
  echo "!! Webhook did not return 200. Response:"
  cat "$RESP_FILE"
  echo
fi
echo

# ── Step 3: Grade ───────────────────────────────────────────────────────────
echo ">> Step 3: grading"
echo
bash benchmarks/grade_level_2.sh "$LOG_START" "$ORIG_LINES"
