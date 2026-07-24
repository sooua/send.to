package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func quietServer(t *testing.T, options ...OptionFn) *Server {
	t.Helper()

	options = append(options, Logger(slog.New(slog.NewJSONHandler(io.Discard, nil))))

	srvr, err := New(options...)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	return srvr
}

func getInternal(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

	return w
}

// The counters belong to the operator, not to whoever found the hostname.
func TestInternalHandlerServesMetrics(t *testing.T) {
	h := quietServer(t).internalHandler()

	res := getInternal(t, h, "/metrics")
	if res.Code != http.StatusOK {
		t.Fatalf("/metrics = %d, want 200", res.Code)
	}

	if !strings.Contains(res.Body.String(), "sendto_uploads_total") {
		t.Errorf("/metrics did not expose the counters: %q", res.Body.String())
	}

	if code := getInternal(t, h, "/health").Code; code != http.StatusOK {
		t.Errorf("/health on the internal listener = %d, want 200", code)
	}
}

// pprof can dump the heap, and on this server the heap holds upload contents.
// It must stay behind --profiler even on a listener that is already internal.
func TestInternalHandlerGatesProfilerBehindTheFlag(t *testing.T) {
	off := quietServer(t).internalHandler()
	if code := getInternal(t, off, "/debug/pprof/").Code; code != http.StatusNotFound {
		t.Errorf("pprof without --profiler = %d, want 404", code)
	}

	on := quietServer(t, EnableProfiler()).internalHandler()
	if code := getInternal(t, on, "/debug/pprof/").Code; code == http.StatusNotFound {
		t.Error("pprof with --profiler is not reachable")
	}
}
