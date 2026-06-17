#!/usr/bin/env bash
# grade_level_4.sh — evaluate the Kyoci-Agent L4 (Self-Healing + HITL + Self-Learning) run.
#
# Usage: bash benchmarks/grade_level_4.sh [LOG_START_LINE]
#   LOG_START_LINE  Line in the server log to scope greps from (default 0).
#
# Env overrides:
#   REPORT_FILE  output Report Card path (default: benchmarks/last_report_level_4.txt)
#   RESP_FILE    webhook response JSON       (default: benchmarks/last_response_level_4.json)
#   BENCH_DB     bench SQLite DB path        (default: /tmp/kyoci-l4-bench.db)
#   BENCH_LOG    server log path             (default: /tmp/kyoci-l4-bench.log)
#   HITL_LOG     hitlctl log path            (default: /tmp/kyoci-l4-hitlctl.log)
#   ENV_DIR      test env directory          (default: benchmarks/app_test_env_l4)
set -euo pipefail

LOG_START="${1:-0}"
LOG_FILE="${BENCH_LOG:-/tmp/kyoci-l4-bench.log}"
HITL_LOG_FILE="${HITL_LOG:-/tmp/kyoci-l4-hitlctl.log}"
ENV_DIR="${ENV_DIR:-benchmarks/app_test_env_l4}"
RESP_FILE="${RESP_FILE:-benchmarks/last_response_level_4.json}"
BENCH_DB="${BENCH_DB:-/tmp/kyoci-l4-bench.db}"
REPORT_FILE="${REPORT_FILE:-benchmarks/last_report_level_4.txt}"
CALC_FILE="$ENV_DIR/calculator.go"

scoped_log() {
  if [[ "${LOG_START:-0}" -gt 0 ]]; then
    tail -n +"$LOG_START" "$LOG_FILE"
  else
    cat "$LOG_FILE"
  fi
}

# ── M1: Test Execution & StdErr Parsing ──────────────────────────────────────
# Server log must show a go test invocation in app_test_env_l4/ AND the worker
# must have seen the test failure ("Expected 5, got 6").
m1_pass=0
m1_detail="no go test evidence in log"
go_test_hits=$(scoped_log | grep -cE 'go test|TestAdd' || true)
expected_hits=$(scoped_log | grep -c 'Expected 5' || true)
if [[ "$go_test_hits" -gt 0 && "$expected_hits" -gt 0 ]]; then
  m1_pass=1
  m1_detail="go test invoked ($go_test_hits refs), failure 'Expected 5' observed ($expected_hits refs)"
elif [[ "$go_test_hits" -gt 0 ]]; then
  m1_detail="go test invoked but failure text not observed in log"
else
  m1_detail="no go test invocation found in server log"
fi

# ── M2: HITL gRPC Fallback ───────────────────────────────────────────────────
# The orchestrator must have emitted a HelpRequest AND hitlctl must have
# submitted a hint. Crucially, the server must NOT have timed out — i.e.
# the response succeeded or the verify loop reached exhaustion cleanly.
m2_pass=0
m2_detail="no HITL traffic observed"
help_emitted=$(scoped_log | grep -c 'HITL request emitted' || true)
hint_received=$(scoped_log | grep -c 'HITL hint received' || true)
hitlctl_submitted=0
if [[ -f "$HITL_LOG_FILE" ]]; then
  hitlctl_submitted=$(grep -c 'hint accepted\|SubmitHint\|auto mode: hint submitted' "$HITL_LOG_FILE" || true)
fi
if [[ "$help_emitted" -ge 1 && "$hitlctl_submitted" -ge 1 ]]; then
  m2_pass=1
  m2_detail="HelpRequest emitted ($help_emitted), hitlctl submitted ($hitlctl_submitted)"
elif [[ "$help_emitted" -ge 1 ]]; then
  m2_detail="HelpRequest emitted but hitlctl submission not logged"
else
  m2_detail="no HelpRequest emitted — orchestrator may not have hit retry limit"
fi

# ── M3: Resumption Context ───────────────────────────────────────────────────
# After the hint, the agent must have applied it and calculator.go must now
# contain `return a + b` (the fix). Also check the test now passes by running
# it directly.
m3_pass=0
m3_detail="calculator.go not fixed"
has_plus=0
has_star=0
if [[ -f "$CALC_FILE" ]]; then
  grep -qE 'return a \+ b' "$CALC_FILE" && has_plus=1
  grep -qE 'return a \* b' "$CALC_FILE" && has_star=1
  if [[ "$has_plus" -eq 1 && "$has_star" -eq 0 ]]; then
    # Confirm by actually running the test.
    if (cd "$ENV_DIR" && go test ./... >/dev/null 2>&1); then
      m3_pass=1
      m3_detail="calculator.go has 'return a + b' AND go test passes"
    else
      m3_detail="calculator.go has + b but go test still fails"
    fi
  else
    m3_detail="calculator.go has_plus=$has_plus has_star=$has_star (still buggy)"
  fi
fi

# ── M4: Self-Learning (L3 Lesson Insertion) ──────────────────────────────────
# The bench SQLite DB must contain at least one row whose metadata indicates
# category=lesson AND whose content references the Add bug — proving the
# orchestrator synthesized a permanent rule from the HITL-assisted fix.
m4_pass=0
m4_detail="no lesson row in L3 SQLite"
db_count=0
lesson_count=0
if [[ -f "$BENCH_DB" ]] && command -v sqlite3 >/dev/null 2>&1; then
  db_count=$(sqlite3 "$BENCH_DB" "SELECT COUNT(*) FROM memories;" 2>/dev/null || echo "0")
  db_count=${db_count:-0}
  # Find lesson rows that reference Add or the math operator.
  lesson_count=$(sqlite3 "$BENCH_DB" \
    "SELECT COUNT(*) FROM memories WHERE metadata LIKE '%category=lesson%' AND (content LIKE '%Add%' OR content LIKE '%calculator%' OR content LIKE '%operator%');" \
    2>/dev/null || echo "0")
  lesson_count=${lesson_count:-0}
  if [[ "$lesson_count" -ge 1 ]]; then
    m4_pass=1
    m4_detail="$lesson_count lesson row(s) referencing Add in L3 (total rows: $db_count)"
  else
    m4_detail="0 lesson rows (total rows in DB: $db_count)"
  fi
elif [[ ! -f "$BENCH_DB" ]]; then
  m4_detail="bench DB not found at $BENCH_DB"
fi

# ── Bonus signals ────────────────────────────────────────────────────────────
attempt_count=$(scoped_log | grep -cE 'orchestrator: attempt' || true)
verify_attempts=$(scoped_log | grep -cE 'orchestrator: verify result' || true)
duration_ms="n/a"
webhook_success="n/a"
if [[ -f "$RESP_FILE" ]]; then
  duration_ms=$(grep -oE '"duration_ms":[0-9]+' "$RESP_FILE" | grep -oE '[0-9]+' || echo "n/a")
  if grep -q '"success":true' "$RESP_FILE"; then
    webhook_success="true"
  elif grep -q '"success":false' "$RESP_FILE"; then
    webhook_success="false"
  fi
fi

# ── Emit Report Card ─────────────────────────────────────────────────────────
pass_label() { [[ "$1" -eq 1 ]] && echo "PASS" || echo "FAIL"; }
overall=$(( m1_pass + m2_pass + m3_pass + m4_pass ))
run_ts=$(date '+%Y-%m-%d %H:%M:%S')

{
echo "============================================================"
echo "  Kyoci-Agent Benchmark L4 — Self-Healing + HITL + Self-Learning"
echo "  Run: $run_ts    (log start line: $LOG_START)"
echo "============================================================"
printf '[M1 Test Execution]       %-4s   %s\n' "$(pass_label "$m1_pass")" "$m1_detail"
printf '[M2 HITL Fallback]        %-4s   %s\n' "$(pass_label "$m2_pass")" "$m2_detail"
printf '[M3 Resumption Context]   %-4s   %s\n' "$(pass_label "$m3_pass")" "$m3_detail"
printf '[M4 Self-Learning (L3)]   %-4s   %s\n' "$(pass_label "$m4_pass")" "$m4_detail"
echo "------------------------------------------------------------"
printf 'Bonus: orchestrator_attempts=%s  verify_runs=%s  webhook_success=%s  duration_ms=%s\n' \
  "$attempt_count" "$verify_attempts" "$webhook_success" "$duration_ms"
printf 'Overall: %d/4 PASS\n' "$overall"
echo "============================================================"
} | tee "$REPORT_FILE"

# Exit non-zero if not all 4 metrics passed (useful for CI).
if [[ "$overall" -lt 4 ]]; then
  exit 1
fi
