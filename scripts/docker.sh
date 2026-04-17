#!/usr/bin/env bash
# One-click docker-compose deploy for send.to.
# Thin wrapper around `docker compose` with readiness probe.
#
# Usage:
#   ./scripts/docker.sh up        # build + start (default)
#   ./scripts/docker.sh down      # stop + remove (keeps volume)
#   ./scripts/docker.sh purge     # stop + remove volume
#   ./scripts/docker.sh logs      # follow logs
#   ./scripts/docker.sh shell     # exec into container (if ever you build
#                                 #   a debug variant with a shell)

set -euo pipefail
cd "$(dirname "$0")/.."

cmd="${1:-up}"
log() { printf '\033[1;34m[docker]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[docker]\033[0m %s\n' "$*" >&2; exit 1; }

command -v docker >/dev/null 2>&1 || die "docker not found"

case "$cmd" in
    up)
        [[ -f .env ]] || { [[ -f .env.example ]] && cp .env.example .env && log "created .env from template"; }
        log "Building image and starting..."
        docker compose up -d --build
        port=$(grep -E "^HOST_PORT=" .env 2>/dev/null | cut -d= -f2 | tr -d '"' | tr -d "'")
        port=${port:-8080}
        log "Waiting for readiness on :$port ..."
        for i in $(seq 1 30); do
            if curl -sS -o /dev/null -w "%{http_code}" "http://127.0.0.1:$port/health.html" 2>/dev/null | grep -q 200; then
                log "Ready — UI http://127.0.0.1:$port/  API same host"
                exit 0
            fi
            sleep 1
        done
        die "service did not become healthy in 30s — check 'docker compose logs'"
        ;;
    down)
        log "Stopping (keeping volume)..."
        docker compose down
        ;;
    purge)
        read -rp "This will delete all uploaded files. Continue? [y/N] " ans
        [[ "$ans" =~ ^[Yy]$ ]] || { log "aborted"; exit 0; }
        docker compose down -v
        ;;
    logs)
        docker compose logs -f
        ;;
    shell)
        docker compose exec sendto sh || die "image has no shell (scratch base); use 'docker compose logs' instead"
        ;;
    *)
        die "unknown command: $cmd (use: up | down | purge | logs | shell)"
        ;;
esac
