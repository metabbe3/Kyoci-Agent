#!/usr/bin/env bash
# run_all.sh — run the full Kyoci-Agent benchmark suite (L1 + L2 + L3) sequentially
# and emit a combined Report Card summary.
#
# Each level's full Report Card is written to benchmarks/last_report_level_N.txt.
# Stdout shows the combined summary only; per-level output is in
# benchmarks/last_run_level_N.log.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

L1_REPORT="$SCRIPT_DIR/last_report_level_1.txt"
L2_REPORT="$SCRIPT_DIR/last_report_level_2.txt"
L3_REPORT="$SCRIPT_DIR/last_report_level_3.txt"
L1_LOG="$SCRIPT_DIR/last_run_level_1.log"
L2_LOG="$SCRIPT_DIR/last_run_level_2.log"
L3_LOG="$SCRIPT_DIR/last_run_level_3.log"

run_ts=$(date '+%Y-%m-%d %H:%M:%S')
echo ">> Kyoci-Agent Benchmark Suite — $run_ts"
echo ">> running Level 1 (Scattered Dependency Bug)..."

REPORT_FILE="$L1_REPORT" \
  bash "$SCRIPT_DIR/run_benchmark.sh" >"$L1_LOG" 2>&1 || true

echo ">> running Level 2 (Spaghetti Refactor)..."

REPORT_FILE="$L2_REPORT" \
  bash "$SCRIPT_DIR/run_level_2.sh" >"$L2_LOG" 2>&1 || true

echo ">> running Level 3 (Memory + MCP + Compaction)..."

REPORT_FILE="$L3_REPORT" \
  bash "$SCRIPT_DIR/run_level_3.sh" >"$L3_LOG" 2>&1 || true

echo
echo ">> per-level logs: $L1_LOG , $L2_LOG , $L3_LOG"
echo

# ── Parse "Overall: N/M PASS" from each report ──────────────────────────────
extract_overall() {
  local report="$1"
  if [[ ! -f "$report" ]]; then
    echo "N/A"
    return
  fi
  local line
  line=$(grep -E '^Overall:' "$report" | tail -1 || true)
  if [[ -z "$line" ]]; then
    echo "N/A"
    return
  fi
  # e.g. "Overall: 4/4 PASS" or "Overall: 5/5 PASS"
  echo "$line" | sed -E 's/^Overall:[[:space:]]*//'
}

l1_score=$(extract_overall "$L1_REPORT")
l2_score=$(extract_overall "$L2_REPORT")
l3_score=$(extract_overall "$L3_REPORT")

# Extract the numerator/denominator for the combined total.
extract_num() { echo "$1" | grep -oE '[0-9]+' | head -1 || echo 0; }
extract_den() { echo "$1" | grep -oE '/[0-9]+' | tr -d '/' || echo 0; }

combined_num=$(( $(extract_num "$l1_score") + $(extract_num "$l2_score") + $(extract_num "$l3_score") ))
combined_den=$(( $(extract_den "$l1_score") + $(extract_den "$l2_score") + $(extract_den "$l3_score") ))

{
echo "============================================================"
echo "  Kyoci-Agent Benchmark Suite"
echo "  Run: $run_ts"
echo "============================================================"
printf 'Level 1 — Scattered Dependency Bug:   %s\n' "$l1_score"
printf 'Level 2 — Spaghetti Refactor:         %s\n' "$l2_score"
printf 'Level 3 — Memory + MCP + Compaction:  %s\n' "$l3_score"
echo "------------------------------------------------------------"
if [[ "$combined_den" -gt 0 ]]; then
  printf 'Combined: %d/%d\n' "$combined_num" "$combined_den"
else
  echo "Combined: N/A"
fi
echo "============================================================"
}

# Dump the latest level's Report Card for visibility.
echo
echo ">> Level 3 Report Card:"
echo
[[ -f "$L3_REPORT" ]] && cat "$L3_REPORT" || echo "(no L3 report)"
