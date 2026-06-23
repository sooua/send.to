#!/usr/bin/env bash
# One-click native deploy for send.to (Linux / macOS / WSL).
#
# Usage:
#   ./scripts/deploy.sh              # build + run in foreground
#   ./scripts/deploy.sh --daemon     # build + run in background (nohup)
#   ./scripts/deploy.sh --stop       # stop a daemonised instance
#   PORT=9000 ./scripts/deploy.sh    # override listening port
#
# Requirements: Go 1.25+, Node 20+, npm, curl.
# Everything is installed into ./build/ and ./data/ — nothing is written
# to system locations.

set -euo pipefail

cd "$(dirname "$0")/.."

MODE="${1:-foreground}"
PORT="${PORT:-18080}"
BUILD_DIR="$(pwd)/build"
DATA_DIR="${DATA_DIR:-$(pwd)/data}"
TMP_DIR="${TMP_DIR:-$(pwd)/data/tmp}"
WEB_DIR="$(pwd)/web/dist"
PID_FILE="$BUILD_DIR/sendto.pid"
LOG_FILE="$BUILD_DIR/sendto.log"
BINARY="$BUILD_DIR/sendto"

log() { printf '\033[1;34m[deploy]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[deploy]\033[0m %s\n' "$*" >&2; exit 1; }

ensure_tool() {
    command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"
}

# --- stop mode ---
if [[ "$MODE" == "--stop" ]]; then
    if [[ -f "$PID_FILE" ]]; then
        pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            log "Stopping pid $pid (graceful, SIGINT)…"
            kill -INT "$pid" || true
            for _ in $(seq 1 30); do
                kill -0 "$pid" 2>/dev/null || break
                sleep 1
            done
            kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null || true
        fi
        rm -f "$PID_FILE"
        log "Stopped."
    else
        log "No PID file at $PID_FILE — nothing to stop."
    fi
    exit 0
fi

# --- preflight ---
ensure_tool go
ensure_tool npm
ensure_tool curl

mkdir -p "$BUILD_DIR" "$DATA_DIR" "$TMP_DIR"

# --- build web ---
log "Building web bundle…"
(
    cd web
    if [[ ! -d node_modules ]]; then
        npm ci --no-audit --no-fund
    fi
    npm run build
)

# --- build server ---
log "Building Go binary ($BINARY)…"
version=$(git describe --tags --always --dirty 2>/dev/null || echo native)
CGO_ENABLED=0 go build \
    -tags netgo \
    -ldflags "-s -w -X github.com/sooua/send.to/cmd.Version=$version" \
    -trimpath \
    -o "$BINARY" .

log "Binary: $(du -h "$BINARY" | cut -f1)  version=$version"

# --- run ---
ARGS=(
    --listener ":$PORT"
    --provider local
    --basedir "$DATA_DIR"
    --temp-path "$TMP_DIR"
    --web-path "$WEB_DIR"
    --shutdown-timeout 30s
)

if [[ "$MODE" == "--daemon" ]]; then
    if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        die "Already running (pid $(cat "$PID_FILE")). Use --stop first."
    fi
    log "Starting daemonised on :$PORT (logs → $LOG_FILE)…"
    nohup "$BINARY" "${ARGS[@]}" >"$LOG_FILE" 2>&1 &
    echo $! >"$PID_FILE"
    sleep 1
    # Readiness probe
    for i in $(seq 1 20); do
        if curl -sS -o /dev/null "http://127.0.0.1:$PORT/health.html"; then
            log "Running (pid $(cat "$PID_FILE")). Health: ✓  UI: http://127.0.0.1:$PORT/"
            exit 0
        fi
        sleep 0.5
    done
    die "Server did not become ready within 10s. Check $LOG_FILE."
fi

log "Starting in foreground on :$PORT (Ctrl+C for graceful shutdown)…"
exec "$BINARY" "${ARGS[@]}"
