#!/usr/bin/env bash
# grade.sh — evaluate the Kyoci-Agent's run against the 4-metric rubric.
#
# Usage: bash benchmarks/grade.sh [LOG_START_LINE]
#   LOG_START_LINE  Line number in /tmp/kyoci-agent.log to scope greps from.
#                   Defaults to 0 (whole file) — useful for manual inspection.
#
# Emits a Report Card to stdout AND writes it to benchmarks/last_report.txt.
set -euo pipefail

LOG_START="${1:-0}"
LOG_FILE="${KYOCI_LOG:-/tmp/kyoci-agent.log}"
ENV_DIR="${ENV_DIR:-agent_test_env}"
RESP_FILE="${RESP_FILE:-benchmarks/last_response.json}"
REPORT_FILE="${REPORT_FILE:-benchmarks/last_report.txt}"

# Scope log reads to lines from LOG_START onward (so re-runs don't double-count).
scoped_log() {
  if [[ "${LOG_START:-0}" -gt 0 ]]; then
    tail -n +"$LOG_START" "$LOG_FILE"
  else
    cat "$LOG_FILE"
  fi
}

# ── M1 + M2: scoped to OUR task's pipeline window ───────────────────────────
# The agent is multi-tenant (Telegram + webhook). Another task may interleave
# between LOG_START and our webhook response. To attribute the right plan and
# worker lines to OUR task, we find our "starting pipeline" line (matched by
# TASK_MARKER — the env dir name appears in our task text), then scan forward
# until the next "starting pipeline" (a different task) or EOF.
TASK_MARKER="${TASK_MARKER:-agent_test_env}"
scoped_pipeline() {
  scoped_log | awk -v marker="$TASK_MARKER" '
    /"msg":"orchestrator: starting pipeline"/ {
      if (in_window) { exit }          # next task started; stop scanning
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
read -r plan_steps tool_total worker_count < <(scoped_pipeline)
plan_steps=${plan_steps:-0}
tool_total=${tool_total:-0}
worker_count=${worker_count:-0}

m1_pass=0; [[ "$plan_steps" -ge 2 ]] && m1_pass=1
m2_pass=0; [[ "$tool_total" -ge 1 ]] && m2_pass=1
m2_strong=0; [[ "$tool_total" -ge 3 ]] && m2_strong=1

# ── M3: Code Accuracy ───────────────────────────────────────────────────────
# connection.js must NOT contain the hardcoded credential, and MUST use the env var.
conn="$ENV_DIR/db/connection.js"
m3_pass=0
m3_detail="missing or incomplete"
if [[ -f "$conn" ]]; then
  has_secret=0
  has_uri=0
  has_envvar=0
  grep -q 'supersecret' "$conn" && has_secret=1 || true
  grep -q 'mongodb://admin' "$conn" && has_uri=1 || true
  grep -q 'process\.env\.DATABASE_URL' "$conn" && has_envvar=1 || true
  if [[ "$has_secret" -eq 0 && "$has_uri" -eq 0 && "$has_envvar" -eq 1 ]]; then
    m3_pass=1
    m3_detail="hardcoded credential removed; process.env.DATABASE_URL present"
  else
    m3_detail="secret=$has_secret uri=$has_uri envvar=$has_envvar"
  fi
fi

# ── M4: File Creation ───────────────────────────────────────────────────────
# .env.example must exist and contain a line starting with DATABASE_URL=
envf="$ENV_DIR/.env.example"
m4_pass=0
m4_detail="missing"
if [[ -f "$envf" ]]; then
  if grep -qE '^DATABASE_URL=' "$envf"; then
    m4_pass=1
    m4_detail=".env.example exists with DATABASE_URL= placeholder"
  else
    m4_detail=".env.example exists but no ^DATABASE_URL= line"
  fi
fi

# ── Parse the webhook response for duration / success ───────────────────────
duration_ms="n/a"
webhook_success="n/a"
if [[ -f "$RESP_FILE" ]]; then
  duration_ms=$(grep -oE '"duration_ms":[0-9]+' "$RESP_FILE" | grep -oE '[0-9]+' || echo "n/a")
  duration_ms=${duration_ms:-n/a}
  if grep -q '"success":true' "$RESP_FILE"; then
    webhook_success="true"
  elif grep -q '"success":false' "$RESP_FILE"; then
    webhook_success="false"
  fi
fi

# ── Emit Report Card ────────────────────────────────────────────────────────
pass_label() { [[ "$1" -eq 1 ]] && echo "PASS" || echo "FAIL"; }

overall=$(( m1_pass + m2_pass + m3_pass + m4_pass ))
run_ts=$(date '+%Y-%m-%d %H:%M:%S')

{
echo "============================================================"
echo "  Kyoci-Agent Benchmark — Scattered Dependency Bug"
echo "  Run: $run_ts    (log start line: $LOG_START)"
echo "============================================================"
printf '[M1 Task Decomposition]   %-4s   plan_steps=%s (>=2 required)\n' \
  "$(pass_label "$m1_pass")" "$plan_steps"
printf '[M2 Tool Usage]           %-4s   tool_calls_total=%s (>=1 required; target >=3 -> %s)\n' \
  "$(pass_label "$m2_pass")" "$tool_total" "$(pass_label "$m2_strong")"
printf '[M3 Code Accuracy]        %-4s   %s\n' \
  "$(pass_label "$m3_pass")" "$m3_detail"
printf '[M4 File Creation]        %-4s   %s\n' \
  "$(pass_label "$m4_pass")" "$m4_detail"
echo "------------------------------------------------------------"
printf 'Overall: %d/4 PASS\n' "$overall"
printf 'Agent duration: %s ms   Webhook success: %s\n' "$duration_ms" "$webhook_success"
echo "============================================================"
} | tee "$REPORT_FILE"
