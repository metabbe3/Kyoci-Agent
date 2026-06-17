#!/usr/bin/env bash
# run_benchmark.sh — orchestrate the Kyoci-Agent scattered-dependency benchmark.
#
# Flow:  preconditions → setup test env → record log offset → trigger agent
#        → wait for synchronous response → grade → emit Report Card.
#
# Re-runnable: cleans agent_test_env/ before each run so the agent always
# starts from the broken state.
set -euo pipefail

# Resolve repo root (this script lives in benchmarks/).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

WEBHOOK_URL="${WEBHOOK_URL:-http://localhost:8080/api/v1/webhook}"
HEALTH_URL="${HEALTH_URL:-http://localhost:8080/health}"
OLLAMA_TAGS_URL="${OLLAMA_TAGS_URL:-http://192.168.2.1:11434/api/tags}"
MODEL_NAME="${MODEL_NAME:-gemma4:12b}"
LOG_FILE="${KYOCI_LOG:-/tmp/kyoci-agent.log}"
TIMEOUT_SECS="${TIMEOUT_SECS:-360}"
ENV_DIR="agent_test_env"

echo ">> Kyoci-Agent Benchmark — Scattered Dependency Bug"
echo ">> repo: $REPO_ROOT"
echo

# ── Preconditions ────────────────────────────────────────────────────────────
echo ">> Checking preconditions..."

if ! curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
  echo "!! FAIL: agent not reachable at $HEALTH_URL"
  echo "   Start it with:  go build -o kyoci-agent ./cmd/server && nohup ./kyoci-agent >$LOG_FILE 2>&1 &"
  exit 1
fi
echo "   agent healthy"

if ! curl -sf "$OLLAMA_TAGS_URL" 2>/dev/null | grep -q "$MODEL_NAME"; then
  echo "!! FAIL: model '$MODEL_NAME' not available at $OLLAMA_TAGS_URL"
  exit 1
fi
echo "   model $MODEL_NAME available"

if [[ ! -f "$LOG_FILE" ]]; then
  echo "!! FAIL: log file $LOG_FILE not found (agent must write logs there)"
  exit 1
fi
echo "   log file readable"

echo

# ── Step 1: Setup — clean + recreate the broken test env ────────────────────
echo ">> Step 1: recreating $ENV_DIR/ in broken state"
rm -rf "$ENV_DIR"
mkdir -p "$ENV_DIR/db" "$ENV_DIR/utils"

cat >"$ENV_DIR/index.js" <<'JS'
const { connect, query } = require('./db/connection');

(async () => {
  await connect();
  const rows = await query('SELECT 1');
  console.log('db ok:', rows);
})();
JS

cat >"$ENV_DIR/db/connection.js" <<'JS'
const { MongoClient } = require('mongodb');

const URI = 'mongodb://admin:supersecret@localhost:27017/prod';
const client = new MongoClient(URI);

async function connect() {
  await client.connect();
  console.log('connected to prod db');
}

async function query(sql) {
  return client.db().collection('misc').find({}).toArray();
}

module.exports = { connect, query };
JS

cat >"$ENV_DIR/utils/logger.js" <<'JS'
function log(msg) {
  console.log(`[app] ${new Date().toISOString()} ${msg}`);
}
module.exports = { log };
JS

echo "   wrote $ENV_DIR/{index.js,db/connection.js,utils/logger.js}"
echo "   hardcoded credential present: $(grep -c supersecret "$ENV_DIR/db/connection.js")"
echo

# ── Step 2: Trigger — record log offset, POST to webhook, wait ──────────────
echo ">> Step 2: triggering agent via webhook (timeout ${TIMEOUT_SECS}s)"
LOG_START=$(wc -l < "$LOG_FILE" | awk '{print $1}')
# Add 1 so grade.sh's tail -n +N starts at the next-written line.
LOG_START=$((LOG_START + 1))
echo "   log start line: $LOG_START"

start_ts=$(date +%s)
HTTP_CODE=$(curl -s -o benchmarks/last_response.json -w "%{http_code}" \
  -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -d @benchmarks/task.json \
  --max-time "$TIMEOUT_SECS") || true
end_ts=$(date +%s)
elapsed=$((end_ts - start_ts))

echo "   webhook returned HTTP $HTTP_CODE in ${elapsed}s"
if [[ "$HTTP_CODE" != "200" ]]; then
  echo "!! Webhook did not return 200. Response:"
  cat benchmarks/last_response.json
  echo
fi
echo

# ── Step 3: Grade ───────────────────────────────────────────────────────────
echo ">> Step 3: grading"
echo
bash benchmarks/grade.sh "$LOG_START"
