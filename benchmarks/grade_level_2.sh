#!/usr/bin/env bash
# grade_level_2.sh — evaluate the Kyoci-Agent's L2 (Spaghetti Refactor) run.
#
# Usage: bash benchmarks/grade_level_2.sh [LOG_START_LINE] [ORIG_SERVER_LINES]
#   LOG_START_LINE     Line in /tmp/kyoci-agent.log to scope greps from (default 0).
#   ORIG_SERVER_LINES  Line count of the pre-refactor server.js (default 23).
#
# Env overrides:
#   REPORT_FILE  output Report Card path (default: benchmarks/last_report_level_2.txt)
#   RESP_FILE    webhook response JSON   (default: benchmarks/last_response_level_2.json)
#   ENV_DIR      test env directory      (default: app_test_env)
#   KYOCI_LOG    agent log path          (default: /tmp/kyoci-agent.log)
set -euo pipefail

LOG_START="${1:-0}"
ORIG_LINES="${2:-23}"
LOG_FILE="${KYOCI_LOG:-/tmp/kyoci-agent.log}"
ENV_DIR="${ENV_DIR:-app_test_env}"
RESP_FILE="${RESP_FILE:-benchmarks/last_response_level_2.json}"
REPORT_FILE="${REPORT_FILE:-benchmarks/last_report_level_2.txt}"

scoped_log() {
  if [[ "${LOG_START:-0}" -gt 0 ]]; then
    tail -n +"$LOG_START" "$LOG_FILE"
  else
    cat "$LOG_FILE"
  fi
}

# ── M1: Task Decomposition (planner must produce >=4 steps) ─────────────────
# Scope to OUR task's pipeline window so interleaved conversations (e.g. a
# Telegram message arriving mid-run) don't pollute the plan-step count.
# The window starts at our "starting pipeline" line (matched by TASK_MARKER —
# the env dir name appears in our task text) and ends at the next "starting
# pipeline" line or EOF.
TASK_MARKER="${TASK_MARKER:-app_test_env}"
scoped_pipeline() {
  scoped_log | awk -v marker="$TASK_MARKER" '
    /"msg":"orchestrator: starting pipeline"/ {
      if (in_window) { exit }
      if ($0 ~ marker) { in_window = 1 }
      next
    }
    in_window && /"msg":"orchestrator: plan produced"/ && plan == "" {
      if (match($0, /"steps":[0-9]+/)) {
        plan = substr($0, RSTART+8, RLENGTH-8)
      }
    }
    in_window && /"msg":"orchestrator: worker done"/ {
      if (match($0, /"tool_calls":[0-9]+/)) {
        total += substr($0, RSTART+13, RLENGTH-13)
        wc++
      }
    }
    END { print (plan ? plan : 0), (total+0), (wc+0) }
  '
}
read -r plan_steps tool_total_pre worker_count < <(scoped_pipeline)
plan_steps=${plan_steps:-0}
m1_pass=0; [[ "$plan_steps" -ge 4 ]] && m1_pass=1

# ── M2: Directory Creation ──────────────────────────────────────────────────
dirs_present=0
for d in routes controllers services; do
  [[ -d "$ENV_DIR/$d" ]] && dirs_present=$((dirs_present + 1))
done
m2_pass=0; [[ "$dirs_present" -eq 3 ]] && m2_pass=1

# ── M3: Clean Entrypoint ────────────────────────────────────────────────────
# server.js must contain the routes require AND be shorter than the original.
srv="$ENV_DIR/server.js"
m3_pass=0
m3_detail="missing server.js"
new_lines=0
has_require=0
if [[ -f "$srv" ]]; then
  new_lines=$(wc -l < "$srv" | awk '{print $1}')
  if grep -q "require(" "$srv" && grep -q "routes/userRoutes" "$srv"; then
    has_require=1
  fi
  if [[ "$has_require" -eq 1 && "$new_lines" -lt "$ORIG_LINES" ]]; then
    m3_pass=1
    m3_detail="server.js=${new_lines} lines (< ${ORIG_LINES} original); require('./routes/userRoutes') present"
  else
    m3_detail="server.js=${new_lines} lines (orig ${ORIG_LINES}); require_present=$has_require"
  fi
fi

# ── M4: Time Limit + success (under 300s) ───────────────────────────────────
duration_ms="n/a"
duration_s="n/a"
webhook_success="n/a"
m4_pass=0
m4_detail="no response file"
if [[ -f "$RESP_FILE" ]]; then
  duration_ms=$(grep -oE '"duration_ms":[0-9]+' "$RESP_FILE" | grep -oE '[0-9]+' || echo "n/a")
  duration_ms=${duration_ms:-n/a}
  if [[ "$duration_ms" != "n/a" ]]; then
    duration_s=$(( duration_ms / 1000 ))
  fi
  if grep -q '"success":true' "$RESP_FILE"; then
    webhook_success="true"
  elif grep -q '"success":false' "$RESP_FILE"; then
    webhook_success="false"
  fi
  if [[ "$webhook_success" == "true" && "$duration_ms" != "n/a" && "$duration_ms" -lt 300000 ]]; then
    m4_pass=1
    m4_detail="duration=${duration_s}s (<300s); webhook success=true"
  else
    m4_detail="duration=${duration_s}s; webhook success=${webhook_success}"
  fi
fi

# ── M5: Documentation ───────────────────────────────────────────────────────
readme="$ENV_DIR/README.md"
m5_pass=0
m5_detail="missing README.md"
if [[ -f "$readme" ]]; then
  # case-insensitive search for 'architecture' or 'layers'
  if grep -qiE 'architecture|layers' "$readme"; then
    m5_pass=1
    m5_detail="README.md exists; mentions 'architecture' / 'layers'"
  else
    m5_detail="README.md exists but no 'architecture'/'layers' mention"
  fi
fi

# ── Bonus signals (printed, not gated) ──────────────────────────────────────
# tool_total is already captured above (scoped to our pipeline window).
tool_total="${tool_total_pre:-0}"

files_created=0
for f in routes/userRoutes.js controllers/userController.js services/userService.js README.md; do
  [[ -f "$ENV_DIR/$f" ]] && files_created=$((files_created + 1))
done

data_moved="no"
if [[ -f "$srv" ]]; then
  if ! grep -q 'let users =' "$srv"; then
    data_moved="yes"
  fi
fi

# ── Emit Report Card ────────────────────────────────────────────────────────
pass_label() { [[ "$1" -eq 1 ]] && echo "PASS" || echo "FAIL"; }
overall=$(( m1_pass + m2_pass + m3_pass + m4_pass + m5_pass ))
run_ts=$(date '+%Y-%m-%d %H:%M:%S')

{
echo "============================================================"
echo "  Kyoci-Agent Benchmark L2 — Spaghetti Refactor"
echo "  Run: $run_ts    (log start line: $LOG_START)"
echo "============================================================"
printf '[M1 Task Decomposition]   %-4s   plan_steps=%s (>=4 required)\n' \
  "$(pass_label "$m1_pass")" "$plan_steps"
printf '[M2 Directory Creation]   %-4s   routes/ + controllers/ + services/ present: %d/3\n' \
  "$(pass_label "$m2_pass")" "$dirs_present"
printf '[M3 Clean Entrypoint]     %-4s   %s\n' \
  "$(pass_label "$m3_pass")" "$m3_detail"
printf '[M4 Time Limit]           %-4s   %s\n' \
  "$(pass_label "$m4_pass")" "$m4_detail"
printf '[M5 Documentation]        %-4s   %s\n' \
  "$(pass_label "$m5_pass")" "$m5_detail"
echo "------------------------------------------------------------"
printf 'Bonus: tool_calls_total=%s   files_created=%d/4   data_array_moved=%s\n' \
  "$tool_total" "$files_created" "$data_moved"
printf 'Overall: %d/5 PASS\n' "$overall"
echo "============================================================"
} | tee "$REPORT_FILE"
