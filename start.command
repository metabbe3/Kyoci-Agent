#!/usr/bin/env bash
# Kyoci Agent — one-click launcher (backend + frontend)
# Double-click this file in Finder, or run `./start.command` in Terminal.

set -uo pipefail

# ── Resolve project dir (works whether double-clicked or run from anywhere) ──
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Pretty colors
BOLD="\033[1m"; GREEN="\033[32m"; CYAN="\033[36m"; YELLOW="\033[33m"; RED="\033[31m"; RESET="\033[0m"
log()  { printf "${CYAN}▶${RESET} %s\n" "$*"; }
ok()   { printf "${GREEN}✓${RESET} %s\n" "$*"; }
warn() { printf "${YELLOW}!${RESET} %s\n" "$*"; }
err()  { printf "${RED}✗${RESET} %s\n" "$*"; }

# ── Dependency checks ────────────────────────────────────────────────────────
command -v go   >/dev/null || { err "Go not installed. Install from https://go.dev/dl/";  read -n1; exit 1; }
command -v node >/dev/null || { err "Node not installed. Install from https://nodejs.org/"; read -n1; exit 1; }
command -v npm  >/dev/null || { err "npm not installed."; read -n1; exit 1; }

BACKEND_PID=""
FRONTEND_PID=""

cleanup() {
    echo
    log "Shutting down…"
    [[ -n "$FRONTEND_PID" ]] && kill "$FRONTEND_PID" 2>/dev/null
    [[ -n "$BACKEND_PID"  ]] && kill "$BACKEND_PID"  2>/dev/null
    wait 2>/dev/null
    ok "Stopped."
}
trap cleanup EXIT INT TERM

# ── Frontend deps ────────────────────────────────────────────────────────────
if [[ ! -d web/node_modules ]]; then
    log "Installing frontend dependencies (first run only)…"
    (cd web && npm install) || { err "npm install failed"; read -n1; exit 1; }
fi

# ── Kill any existing instances (prevents port conflicts) ─────────────────────
pkill -f "cmd/server" 2>/dev/null; sleep 1

# ── Rebuild frontend (ensures embedded SPA is current) ────────────────────────
log "Building frontend…"
(cd web && npm run build --silent) 2>/dev/null && ok "Frontend built" || warn "Frontend build skipped"

# ── Clear Go cache (forces fresh embed of web/dist/) ──────────────────────────
log "Clearing Go build cache…"
go clean -cache

# ── Start backend (HITL disabled to avoid gRPC port conflicts) ───────────────
log "Starting Go backend on :8080 …"
KYOCI_HITL_ENABLED=false go run ./cmd/server >"$SCRIPT_DIR/backend.log" 2>&1 &
BACKEND_PID=$!

# ── Wait for backend health ──────────────────────────────────────────────────
log "Waiting for backend to be ready…"
backend_ready=false
for i in {1..30}; do
    if curl -sf http://localhost:8080/health >/dev/null 2>&1; then
        ok "Backend ready (pid $BACKEND_PID)"
        backend_ready=true
        break
    fi
    if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
        err "Backend crashed during startup. Last lines of backend.log:"
        tail -n 20 "$SCRIPT_DIR/backend.log"
        read -n1
        exit 1
    fi
    sleep 0.5
done
if [[ "$backend_ready" != "true" ]]; then
    warn "Backend still warming up after 15s — continuing anyway."
fi

# ── Start frontend (Vite dev server on :5173) ────────────────────────────────
log "Starting frontend dev server on :5173 …"
(cd web && npm run dev) >"$SCRIPT_DIR/frontend.log" 2>&1 &
FRONTEND_PID=$!

# ── Wait for Vite ────────────────────────────────────────────────────────────
for i in {1..30}; do
    if curl -sf http://localhost:5173 >/dev/null 2>&1; then
        ok "Frontend ready (pid $FRONTEND_PID)"
        break
    fi
    sleep 0.5
done

# ── Open browser ─────────────────────────────────────────────────────────────
URL="http://localhost:5173"
log "Opening $URL in browser …"
sleep 1
( open "$URL" >/dev/null 2>&1 || xdg-open "$URL" >/dev/null 2>&1 ) &

echo
printf "${BOLD}╔══════════════════════════════════════════════════╗\n${RESET}"
printf "${BOLD}║       Kyoci Agent is running                     ║\n${RESET}"
printf "${BOLD}╚══════════════════════════════════════════════════╝\n${RESET}"
echo
printf "  Frontend : ${GREEN}http://localhost:5173${RESET}\n"
printf "  Backend  : ${GREEN}http://localhost:8080${RESET}\n"
printf "  Health   : ${GREEN}http://localhost:8080/health${RESET}\n"
echo
printf "  Logs     : ${CYAN}backend.log${RESET} / ${CYAN}frontend.log${RESET}\n"
echo
printf "  Press ${BOLD}Ctrl+C${RESET} in this window to stop both servers.\n"
echo

# Keep script alive so background jobs survive until Ctrl+C
wait
