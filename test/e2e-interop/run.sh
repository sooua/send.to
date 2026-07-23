#!/usr/bin/env bash
#
# Proves the Go and browser implementations of the end-to-end encryption format
# produce and accept the same bytes, in both directions, across the sizes where
# a chunked AEAD tends to break.
#
# Requires Go and Node 22+. Run from the repository root:
#
#   bash test/e2e-interop/run.sh
#
set -euo pipefail

cd "$(dirname "$0")/../.."

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

echo "building the Go side..."
CGO_ENABLED=0 go build -o "$work/interop" ./test/e2e-interop

echo "compiling the browser side..."
npx --yes --prefix web esbuild web/src/lib/e2e.ts \
    --format=esm --platform=neutral --log-level=warning \
    --outfile="$work/e2e.mjs"

export E2E_MODULE="$work/e2e.mjs"

# Chunk size is 64 KiB, so cover both sides of every boundary.
sizes=(0 1 1000 65535 65536 65537 131072 200000)
failures=0

check() {
    local label="$1" expected="$2" actual="$3"
    if cmp -s "$expected" "$actual"; then
        printf '  %-28s ok\n' "$label"
    else
        printf '  %-28s MISMATCH\n' "$label"
        failures=$((failures + 1))
    fi
}

for size in "${sizes[@]}"; do
    echo "size ${size}:"

    plain="$work/plain-$size"
    head -c "$size" /dev/urandom > "$plain"

    key="$("$work/interop" genkey)"

    # Go encrypts, the browser implementation decrypts.
    "$work/interop" encrypt "$key" "sample-$size.bin" "$plain" "$work/go.enc"
    meta="$(node test/e2e-interop/interop.mjs decrypt "$key" "$work/go.enc" "$work/go-then-js.out")"
    check "go encrypt -> js decrypt" "$plain" "$work/go-then-js.out"

    if [ "$meta" != "{\"name\":\"sample-$size.bin\",\"type\":\"application/octet-stream\"}" ]; then
        printf '  %-28s METADATA MISMATCH: %s\n' "metadata" "$meta"
        failures=$((failures + 1))
    fi

    # The browser implementation encrypts, Go decrypts.
    node test/e2e-interop/interop.mjs encrypt "$key" "sample-$size.bin" "$plain" "$work/js.enc"
    "$work/interop" decrypt "$key" "$work/js.enc" "$work/js-then-go.out" > /dev/null
    check "js encrypt -> go decrypt" "$plain" "$work/js-then-go.out"

    # Both implementations must agree on the exact ciphertext length, or an
    # upload cannot declare a Content-Length.
    go_size=$(wc -c < "$work/go.enc")
    js_size=$(wc -c < "$work/js.enc")
    if [ "$go_size" != "$js_size" ]; then
        printf '  %-28s LENGTH MISMATCH: go=%s js=%s\n' "ciphertext length" "$go_size" "$js_size"
        failures=$((failures + 1))
    else
        printf '  %-28s ok (%s bytes)\n' "ciphertext length" "$go_size"
    fi
done

echo
echo "cross-implementation rejection:"

# A key the file was not encrypted with must fail on both sides.
wrong="$("$work/interop" genkey)"
if node test/e2e-interop/interop.mjs decrypt "$wrong" "$work/go.enc" "$work/nope" 2>/dev/null; then
    echo "  js accepted the wrong key       FAIL"
    failures=$((failures + 1))
else
    echo "  js rejects the wrong key        ok"
fi

if "$work/interop" decrypt "$wrong" "$work/js.enc" "$work/nope" 2>/dev/null; then
    echo "  go accepted the wrong key       FAIL"
    failures=$((failures + 1))
else
    echo "  go rejects the wrong key        ok"
fi

# Truncation must be detected, not read back as a shorter file.
head -c $(( $(wc -c < "$work/go.enc") - 100 )) "$work/go.enc" > "$work/truncated.enc"
if node test/e2e-interop/interop.mjs decrypt "$key" "$work/truncated.enc" "$work/nope" 2>/dev/null; then
    echo "  js accepted a truncated file    FAIL"
    failures=$((failures + 1))
else
    echo "  js rejects a truncated file     ok"
fi

echo
if [ "$failures" -ne 0 ]; then
    echo "$failures check(s) failed"
    exit 1
fi

echo "all interop checks passed"
