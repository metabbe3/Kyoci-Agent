#!/usr/bin/env bash
# grade_level_3.sh — evaluate the Kyoci-Agent L3 (Memory + MCP + Compaction) run.
#
# Usage: bash benchmarks/grade_level_3.sh [LOG_START_LINE]
#   LOG_START_LINE  Line in the bench agent log to scope greps from (default 0).
#
# Env overrides:
#   REPORT_FILE  output Report Card path (default: benchmarks/last_report_level_3.txt)
#   RESP_B       Session B response JSON  (default: benchmarks/last_response_level_3_b.json)
#   ENV_DIR      test env directory       (default: app_test_env)
#   BENCH_DB     bench SQLite DB path     (default: /tmp/kyoci-l3-bench.db)
#   BENCH_LOG    bench agent log path     (default: /tmp/kyoci-l3-bench.log)
set -euo pipefail

LOG_START="${1:-0}"
LOG_FILE="${BENCH_LOG:-/tmp/kyoci-l3-bench.log}"
ENV_DIR="${ENV_DIR:-app_test_env}"
RESP_B="${RESP_B:-benchmarks/last_response_level_3_b.json}"
BENCH_DB="${BENCH_DB:-/tmp/kyoci-l3-bench.db}"
REPORT_FILE="${REPORT_FILE:-benchmarks/last_report_level_3.txt}"

main_go="$ENV_DIR/main.go"

scoped_log() {
  if [[ "${LOG_START:-0}" -gt 0 ]]; then
    tail -n +"$LOG_START" "$LOG_FILE"
  else
    cat "$LOG_FILE"
  fi
}

# ── M1: L3 Memory Retrieval ──────────────────────────────────────────────────
# main.go must use net/http (not gin/fiber/echo) and have snake_case json tags,
# proving Session A's preferences leaked through L3 memory into Session B's output.
m1_pass=0
m1_detail="main.go missing or incomplete"
if [[ -f "$main_go" ]]; then
  has_nethttp=0; has_snake=0; has_framework=0
  grep -q '"net/http"' "$main_go" && has_nethttp=1
  # snake_case json tags: json:"some_field_name"
  grep -qE 'json:"[a-z]+_[a-z]+"' "$main_go" && has_snake=1
  # frameworks that should NOT appear (Session A said "no frameworks")
  grep -qiE 'gin-gin|fiber|labstack/echo|gin-gonic|gorilla/mux' "$main_go" && has_framework=1
  if [[ "$has_nethttp" -eq 1 && "$has_snake" -eq 1 && "$has_framework" -eq 0 ]]; then
    m1_pass=1
    m1_detail="net/http=yes, snake_case_json=yes, frameworks=no"
  else
    m1_detail="net/http=$has_nethttp, snake_case=$has_snake, frameworks=$has_framework"
  fi
fi

# ── M2: MCP Tool Execution ───────────────────────────────────────────────────
# main.go must contain all three schema fields from the MCP mock server.
m2_pass=0
m2_detail="main.go missing or missing schema fields"
if [[ -f "$main_go" ]]; then
  fields_found=0
  for field in uuid email_address created_at; do
    grep -q "$field" "$main_go" && fields_found=$((fields_found + 1))
  done
  if [[ "$fields_found" -eq 3 ]]; then
    m2_pass=1
    m2_detail="all 3 schema fields present (uuid, email_address, created_at)"
  else
    m2_detail="schema fields found: $fields_found/3"
  fi
fi

# ── M3: Auto-Compaction / L3 Persistence ─────────────────────────────────────
# The bench SQLite DB must have >=1 long-term entry from Session A (either via
# explicit remember tool or auto-compaction).
m3_pass=0
m3_detail="no entries in bench DB"
db_count=0
if [[ -f "$BENCH_DB" ]] && command -v sqlite3 >/dev/null 2>&1; then
  db_count=$(sqlite3 "$BENCH_DB" "SELECT COUNT(*) FROM memories;" 2>/dev/null || echo "0")
  db_count=${db_count:-0}
  if [[ "$db_count" -gt 0 ]]; then
    m3_pass=1
    m3_detail="$db_count entries in L3 SQLite (bench DB)"
  else
    m3_detail="0 entries — remember tool + compaction both failed"
  fi
elif [[ ! -f "$BENCH_DB" ]]; then
  m3_detail="bench DB not found at $BENCH_DB"
fi

# ── M4: Time Limit + success ─────────────────────────────────────────────────
# Session B must succeed and complete under 300s.
duration_ms="n/a"
duration_s="n/a"
webhook_success="n/a"
m4_pass=0
m4_detail="no Session B response file"
if [[ -f "$RESP_B" ]]; then
  duration_ms=$(grep -oE '"duration_ms":[0-9]+' "$RESP_B" | grep -oE '[0-9]+' || echo "n/a")
  duration_ms=${duration_ms:-n/a}
  if [[ "$duration_ms" != "n/a" ]]; then
    duration_s=$(( duration_ms / 1000 ))
  fi
  if grep -q '"success":true' "$RESP_B"; then
    webhook_success="true"
  elif grep -q '"success":false' "$RESP_B"; then
    webhook_success="false"
  fi
  if [[ "$webhook_success" == "true" && "$duration_ms" != "n/a" && "$duration_ms" -lt 300000 ]]; then
    m4_pass=1
    m4_detail="duration=${duration_s}s (<300s); webhook success=true"
  else
    m4_detail="duration=${duration_s}s; webhook success=${webhook_success}"
  fi
fi

# ── M5: Zero-Hallucination ───────────────────────────────────────────────────
# The User struct must have EXACTLY 3 fields matching the MCP schema. No
# invented fields like name, age, password, username, phone.
m5_pass=0
m5_detail="main.go missing"
if [[ -f "$main_go" ]]; then
  # Count how many of the 3 schema fields appear as struct/json fields.
  expected_count=0
  for field in uuid email_address created_at; do
    grep -q "$field" "$main_go" && expected_count=$((expected_count + 1))
  done
  # Check for common hallucinated fields that are NOT in the schema.
  hallucinated=0
  for field in '"name"' 'Name\s' '"age"' 'Age\s' '"password"' 'Password\s' '"username"' '"phone"' '"address"'; do
    if grep -qE "$field" "$main_go" 2>/dev/null; then
      hallucinated=$((hallucinated + 1))
    fi
  done
  if [[ "$expected_count" -eq 3 && "$hallucinated" -eq 0 ]]; then
    m5_pass=1
    m5_detail="exactly 3 schema fields, 0 hallucinated"
  else
    m5_detail="schema_fields=$expected_count/3, hallucinated=$hallucinated"
  fi
fi

# ── Bonus signals ────────────────────────────────────────────────────────────
# remember tool called in Session A
remember_called=$(scoped_log | grep -c 'remember' || true)
# MCP tool called in Session B
mcp_called=$(scoped_log | grep -cE 'kyoci_fetch_user_schema|fetch_user_schema' || true)
# plan steps for Session B
plan_steps=$(scoped_log | grep -oE '"steps":[0-9]+' | tail -1 | grep -oE '[0-9]+' || echo "0")
files_created=0
for f in main.go go.mod; do
  [[ -f "$ENV_DIR/$f" ]] && files_created=$((files_created + 1))
done

# ── Emit Report Card ────────────────────────────────────────────────────────
pass_label() { [[ "$1" -eq 1 ]] && echo "PASS" || echo "FAIL"; }
overall=$(( m1_pass + m2_pass + m3_pass + m4_pass + m5_pass ))
run_ts=$(date '+%Y-%m-%d %H:%M:%S')

{
echo "============================================================"
echo "  Kyoci-Agent Benchmark L3 — Memory + MCP + Compaction"
echo "  Run: $run_ts    (log start line: $LOG_START)"
echo "============================================================"
printf '[M1 L3 Memory Retrieval]  %-4s   %s\n' \
  "$(pass_label "$m1_pass")" "$m1_detail"
printf '[M2 MCP Tool Execution]   %-4s   %s\n' \
  "$(pass_label "$m2_pass")" "$m2_detail"
printf '[M3 Auto-Compaction/L3]   %-4s   %s\n' \
  "$(pass_label "$m3_pass")" "$m3_detail"
printf '[M4 Time Limit]           %-4s   %s\n' \
  "$(pass_label "$m4_pass")" "$m4_detail"
printf '[M5 Zero-Hallucination]   %-4s   %s\n' \
  "$(pass_label "$m5_pass")" "$m5_detail"
echo "------------------------------------------------------------"
printf 'Bonus: remember_calls=%s   mcp_calls=%s   plan_steps=%s   files_created=%d/2\n' \
  "$remember_called" "$mcp_called" "$plan_steps" "$files_created"
printf 'Overall: %d/5 PASS\n' "$overall"
echo "============================================================"
} | tee "$REPORT_FILE"
