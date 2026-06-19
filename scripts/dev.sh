#!/usr/bin/env bash
# scripts/dev.sh — canonical dev runner.
#
# Starts the Go backend and the Vite frontend, redirecting each process's
# combined stdout/stderr into a date-partitioned folder under logs/. The
# folder name is the current date (UTC-ish local), so a single day's runs
# accumulate in one place; restarting on the same day appends to the same
# files.
#
# Usage:
#   scripts/dev.sh                # both servers, foreground
#   scripts/dev.sh backend        # backend only
#   scripts/dev.sh frontend       # frontend only
#
# Ctrl-C (or kill $BACKEND_PID $FRONTEND_PID) cleans up both processes via
# the EXIT trap. Logs land at:
#   logs/<YYYY-MM-DD>/backend.log
#   logs/<YYYY-MM-DD>/frontend.log
# Per-run agent execution traces (separate from these dev-server logs) live
# alongside as run_<task_id>.log, written by the Go app itself.
set -euo pipefail

cd "$(dirname "$0")/.."

DATE="$(date +%Y-%m-%d)"
LOG_DIR="logs/$DATE"
mkdir -p "$LOG_DIR"

mode="${1:-all}"
backend_pid=""
frontend_pid=""

cleanup() {
	[[ -n "$backend_pid"  ]] && kill "$backend_pid"  2>/dev/null || true
	[[ -n "$frontend_pid" ]] && kill "$frontend_pid" 2>/dev/null || true
	wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo "→ logs writing to $LOG_DIR/"

if [[ "$mode" == "all" || "$mode" == "backend" ]]; then
	pkill -f "cmd/server" 2>/dev/null; sleep 1
	echo "→ rebuilding frontend + clearing Go cache (fresh embed)…"
	(cd web && npm run build --silent) 2>/dev/null
	go clean -cache
	echo "→ starting backend (go run ./cmd/server) → $LOG_DIR/backend.log"
	KYOCI_HITL_ENABLED=false go run ./cmd/server > "$LOG_DIR/backend.log" 2>&1 &
	backend_pid=$!
	echo "  backend pid: $backend_pid"
fi

if [[ "$mode" == "all" || "$mode" == "frontend" ]]; then
	if [[ -d web ]]; then
		echo "→ starting frontend (npm run dev in web/) → $LOG_DIR/frontend.log"
		( cd web && npm run dev ) > "$LOG_DIR/frontend.log" 2>&1 &
		frontend_pid=$!
		echo "  frontend pid: $frontend_pid"
	else
		echo "→ web/ not found; skipping frontend"
	fi
fi

# Block until either child exits. The trap cleans up the survivor.
wait
