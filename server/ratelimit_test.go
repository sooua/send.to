package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiterAllowsBurstThenBlocks(t *testing.T) {
	rl := newIPRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.allow("10.0.0.1") {
			t.Fatalf("request %d should be allowed within the burst", i)
		}
	}

	if rl.allow("10.0.0.1") {
		t.Error("fourth request should be rate limited")
	}

	// A different source address has its own bucket.
	if !rl.allow("10.0.0.2") {
		t.Error("a different IP should not be affected")
	}
}

// The limiter map must not grow without bound: idle buckets are swept.
func TestIPRateLimiterEvictsIdleEntries(t *testing.T) {
	rl := newIPRateLimiter(1, time.Minute)

	rl.allow("10.0.0.1")
	rl.allow("10.0.0.2")

	rl.mu.Lock()
	if len(rl.limiters) != 2 {
		rl.mu.Unlock()
		t.Fatalf("expected 2 buckets, got %d", len(rl.limiters))
	}
	// Age both entries and the last sweep past the TTL.
	stale := time.Now().Add(-2 * rateLimiterIdleTTL)
	for _, e := range rl.limiters {
		e.lastSeen = stale
	}
	rl.lastGC = stale
	rl.mu.Unlock()

	// The next request triggers the sweep and keeps only its own bucket.
	rl.allow("10.0.0.3")

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if len(rl.limiters) != 1 {
		t.Errorf("expected idle buckets to be evicted, %d remain", len(rl.limiters))
	}
	if _, ok := rl.limiters["10.0.0.3"]; !ok {
		t.Error("the active bucket should have been kept")
	}
}

// Every rate-limited route shares one budget rather than getting its own.
func TestRateLimitSharedAcrossRoutes(t *testing.T) {
	srvr, err := New(
		RateLimit(2),
		Logger(slog.New(slog.NewJSONHandler(io.Discard, nil))),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	srvr.rateLimiter = newIPRateLimiter(2, time.Minute)

	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	upload := srvr.rateLimit(ok)
	download := srvr.rateLimit(ok)

	call := func(h http.HandlerFunc) int {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.9:1234"
		w := httptest.NewRecorder()
		h(w, req)
		return w.Result().StatusCode
	}

	if code := call(upload); code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", code)
	}
	if code := call(download); code != http.StatusOK {
		t.Fatalf("second request = %d, want 200", code)
	}
	if code := call(download); code != http.StatusTooManyRequests {
		t.Errorf("third request = %d, want 429 — routes must share one budget", code)
	}
}

// With no limit configured the wrapper is a pass-through.
func TestRateLimitDisabled(t *testing.T) {
	srvr, err := New(Logger(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	h := srvr.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 50; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.9:1234"
		w := httptest.NewRecorder()
		h(w, req)

		if code := w.Result().StatusCode; code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200", i, code)
		}
	}
}
