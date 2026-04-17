#!/usr/bin/env bash
# End-to-end smoke test for send.to.
# Starts the server via a supervisor on a random port, exercises every
# endpoint we care about, and sends a CTRL_BREAK (Windows) or SIGINT
# (POSIX) to verify graceful shutdown. Exits non-zero on any failure.
set -u

FAIL=0
PORT=$(( 20000 + RANDOM % 20000 ))
STORE=$(mktemp -d)
TMP=$(mktemp -d)
LOG=$(mktemp)
BIN=${BIN:-./send.to.exe}
RUNSERVER=${RUNSERVER:-./runserver.exe}

pass() { printf '  \033[32m✓\033[0m %s\n' "$1"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$1"; FAIL=1; }
step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

cleanup() {
    if [[ -n "${SUPERVISOR_PID:-}" ]] && kill -0 "$SUPERVISOR_PID" 2>/dev/null; then
        kill -9 "$SUPERVISOR_PID" 2>/dev/null || true
    fi
    if [[ -n "${CHILD_PID:-}" ]] && kill -0 "$CHILD_PID" 2>/dev/null; then
        kill -9 "$CHILD_PID" 2>/dev/null || true
    fi
    # On Windows, bash's kill may not reach a detached process group — use taskkill as safety net.
    if [[ -n "${CHILD_PID:-}" ]]; then
        powershell.exe -NoProfile -Command "Stop-Process -Id $CHILD_PID -Force -ErrorAction SilentlyContinue" 2>/dev/null || true
    fi
    rm -rf "$STORE" "$TMP"
}
trap cleanup EXIT

step "Starting server via supervisor on port $PORT"
"$RUNSERVER" "$BIN" \
    --listener "127.0.0.1:$PORT" \
    --provider local \
    --basedir "$STORE" \
    --temp-path "$TMP" \
    --rate-limit 0 \
    --max-upload-size 1 \
    --shutdown-timeout 10s \
    >"$LOG" 2>&1 &
SUPERVISOR_PID=$!

# Discover the child (server) pid and control port printed by the supervisor.
for i in $(seq 1 20); do
    CHILD_PID=$(grep -oE "CHILD_PID=[0-9]+" "$LOG" 2>/dev/null | head -1 | cut -d= -f2)
    CTRL_PORT=$(grep -oE "CTRL_PORT=[0-9]+" "$LOG" 2>/dev/null | head -1 | cut -d= -f2)
    [[ -n "${CHILD_PID:-}" && -n "${CTRL_PORT:-}" ]] && break
    sleep 0.2
done
if [[ -z "${CHILD_PID:-}" || -z "${CTRL_PORT:-}" ]]; then
    fail "supervisor did not report child pid and ctrl port"
    cat "$LOG"
    exit 1
fi
printf '  supervisor=%s child=%s ctrl=:%s log=%s\n' "$SUPERVISOR_PID" "$CHILD_PID" "$CTRL_PORT" "$LOG"

# Wait for readiness (up to 8s)
for i in $(seq 1 40); do
    if curl -sS -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/health.html" 2>/dev/null | grep -q 200; then
        break
    fi
    sleep 0.2
done
if ! curl -sS -o /dev/null "http://127.0.0.1:$PORT/health.html"; then
    fail "server did not become ready"
    cat "$LOG"
    exit 1
fi
pass "server ready on :$PORT (pid $CHILD_PID)"

step "1. Health check"
code=$(curl -sS -o /dev/null -w "%{http_code}" "http://127.0.0.1:$PORT/health.html")
[[ "$code" == "200" ]] && pass "GET /health.html → 200" || fail "got $code"

step "2. Security headers"
hdrs=$(curl -sS -I "http://127.0.0.1:$PORT/health.html")
for h in "Content-Security-Policy" "X-Content-Type-Options" "X-Frame-Options" \
         "Cross-Origin-Opener-Policy" "Cross-Origin-Resource-Policy" \
         "Referrer-Policy" "Permissions-Policy"; do
    if echo "$hdrs" | grep -qi "^$h:"; then pass "$h present"; else fail "$h missing"; fi
done

step "3. Upload / download / delete round-trip"
echo "hello $(date +%s)" >"$TMP/payload.txt"
UPLOAD=$(curl -sS -D "$TMP/up.h" --upload-file "$TMP/payload.txt" "http://127.0.0.1:$PORT/greeting.txt")
if [[ "$UPLOAD" =~ ^http://127\.0\.0\.1:$PORT/ ]]; then pass "PUT → $UPLOAD"; else fail "bad upload response: $UPLOAD"; fi
DEL=$(grep -i "^x-url-delete:" "$TMP/up.h" | awk '{print $2}' | tr -d '\r\n')
[[ -n "$DEL" ]] && pass "X-Url-Delete returned" || fail "X-Url-Delete missing"
grep -qi "^cache-control: no-store" "$TMP/up.h" && pass "Cache-Control: no-store set" || fail "no-store missing"

body=$(curl -sS "$UPLOAD")
[[ "$body" == "$(cat "$TMP/payload.txt")" ]] && pass "GET returns identical bytes" || fail "content mismatch"

code=$(curl -sS -o /dev/null -w "%{http_code}" -X DELETE "$DEL")
[[ "$code" == "200" ]] && pass "DELETE → 200" || fail "DELETE got $code"
code=$(curl -sS -o /dev/null -w "%{http_code}" "$UPLOAD")
[[ "$code" == "404" ]] && pass "post-delete GET → 404" || fail "post-delete got $code"

step "4. Encryption round-trip"
ENCU=$(curl -sS -H "X-Encrypt-Password: secret" --upload-file "$TMP/payload.txt" "http://127.0.0.1:$PORT/enc.txt")
raw=$(curl -sS "$ENCU" | head -1)
[[ "$raw" == "-----BEGIN PGP MESSAGE-----" ]] && pass "stored as PGP armor" || fail "not encrypted: $raw"
dec=$(curl -sS -H "X-Decrypt-Password: secret" "$ENCU")
[[ "$dec" == "$(cat "$TMP/payload.txt")" ]] && pass "correct password decrypts" || fail "decryption mismatch"
code=$(curl -sS -o /dev/null -w "%{http_code}" -H "X-Decrypt-Password: nope" "$ENCU")
[[ "$code" == "500" ]] && pass "wrong password → 500" || fail "wrong password got $code"

step "5. Max-upload-size enforcement (limit = 1KB)"
head -c 4096 /dev/urandom >"$TMP/big.bin"
code=$(curl -sS -o /dev/null -w "%{http_code}" --upload-file "$TMP/big.bin" "http://127.0.0.1:$PORT/big.bin")
if [[ "$code" != "200" && "$code" != "201" ]]; then
    pass "4KB upload rejected (status $code)"
else
    fail "oversize upload not rejected (status $code)"
fi

step "6. Max-Downloads header"
LIM=$(curl -sS -H "Max-Downloads: 2" --upload-file "$TMP/payload.txt" "http://127.0.0.1:$PORT/limited.txt")
c1=$(curl -sS -o /dev/null -w "%{http_code}" "$LIM")
c2=$(curl -sS -o /dev/null -w "%{http_code}" "$LIM")
c3=$(curl -sS -o /dev/null -w "%{http_code}" "$LIM")
[[ "$c1" == "200" && "$c2" == "200" && "$c3" == "404" ]] && pass "2-download cap honored ($c1/$c2/$c3)" || fail "cap failed ($c1/$c2/$c3)"

step "7. CORS preflight"
cors=$(curl -sS -I -X OPTIONS -H "Origin: http://example.com" -H "Access-Control-Request-Method: PUT" "http://127.0.0.1:$PORT/foo")
echo "$cors" | grep -qi "^access-control-allow-origin: http://example.com" && pass "Allow-Origin echoed" || fail "Allow-Origin missing"
echo "$cors" | grep -qi "X-Decrypt-Password" && pass "X-Decrypt-Password in allow-headers" || fail "X-Decrypt-Password not exposed"

step "8. Concurrent uploads (10 in parallel via xargs)"
rm -f "$TMP/urls.txt"
# %{url_effective}\n ensures one line per response
seq 1 10 | xargs -I{} -P 10 -n 1 \
    curl -sS -w "\n" --upload-file "$TMP/payload.txt" "http://127.0.0.1:$PORT/c{}.txt" \
    >>"$TMP/urls.txt" 2>&1
unique=$(tr -d '\r' <"$TMP/urls.txt" | grep -c "^http://127\.0\.0\.1:$PORT/")
if [[ "$unique" == "10" ]]; then
    pass "10 distinct URLs returned"
else
    fail "got $unique/10 URLs"
    head -5 "$TMP/urls.txt"
fi

step "9. Graceful shutdown"
# Build a 512 KB payload so the upload spans hundreds of ms.
head -c 524288 /dev/urandom >"$TMP/slow.bin"
# Temporarily lift the 1 KB cap on this instance? We can't — so use a
# payload that is within the cap but throttle the upload via curl
# --limit-rate to stretch it across the shutdown event.
head -c 512 /dev/urandom >"$TMP/slow.bin"
(
    # --limit-rate 256 bytes/sec → 512 byte payload takes ~2s.
    curl -sS --limit-rate 256 --upload-file "$TMP/slow.bin" \
        "http://127.0.0.1:$PORT/last.txt" >"$TMP/last.out" 2>"$TMP/last.err"
    echo $? >"$TMP/last.rc"
) &
LAST_BG=$!
# Wait for the upload to actually enter the server (look for log line).
for i in $(seq 1 20); do
    grep -q '"filename":"last.txt"' "$LOG" 2>/dev/null && break
    sleep 0.1
done

# Trigger graceful shutdown by opening a TCP connection to the
# supervisor's control port. On Windows this is more reliable than bash's
# kill -INT which does not produce a real CTRL_C_EVENT for Go console apps.
curl -sS -m 2 "http://127.0.0.1:$CTRL_PORT/" >/dev/null 2>&1 || true

for i in $(seq 1 30); do
    kill -0 "$SUPERVISOR_PID" 2>/dev/null || break
    sleep 0.5
done
if kill -0 "$SUPERVISOR_PID" 2>/dev/null; then
    fail "supervisor did not exit within 15s"
    kill -9 "$SUPERVISOR_PID" 2>/dev/null
else
    pass "supervisor exited after signal"
fi

wait "$LAST_BG" 2>/dev/null || true
if [[ -f "$TMP/last.rc" ]] && [[ "$(cat "$TMP/last.rc")" == "0" ]]; then
    pass "in-flight upload completed during graceful shutdown"
else
    fail "in-flight upload rc=$(cat "$TMP/last.rc" 2>/dev/null) err=$(cat "$TMP/last.err" 2>/dev/null | head -1)"
fi

grep -q '"msg":"Shutting down server"' "$LOG" && pass "logged 'Shutting down server'" || fail "shutdown log missing"
grep -q '"msg":"Server stopped"' "$LOG"       && pass "logged 'Server stopped'"       || fail "final log missing"

step "Summary"
if [[ "$FAIL" == "0" ]]; then
    printf '\033[1;32mALL TESTS PASSED\033[0m\n'
    exit 0
else
    printf '\033[1;31mSOME TESTS FAILED\033[0m — server log follows:\n\n'
    cat "$LOG"
    exit 1
fi
