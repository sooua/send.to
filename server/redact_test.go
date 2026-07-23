package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRedactPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"download", "/aB3cD4eF12/notes.md", "/aB********/notes.md"},
		{"download with action", "/get/aB3cD4eF12/notes.md", "/get/aB********/notes.md"},
		{"inline action", "/inline/aB3cD4eF12/notes.md", "/inline/aB********/notes.md"},
		{"deletion token", "/aB3cD4eF12/notes.md/9xKq7", "/aB********/notes.md/9x***"},
		{"archive", "/(tokA/a.txt,tokB/b.txt).zip", "/(...).zip"},
		{"archive tar.gz", "/(tokA/a.txt).tar.gz", "/(...).tar.gz"},
		// Single-segment paths are uploads or static assets, not secrets.
		{"upload", "/notes.md", "/notes.md"},
		{"root", "/", "/"},
		{"health", "/health.html", "/health.html"},
		{"static asset", "/_astro/index.abc123.js", "/_a****/index.abc123.js"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactPath(tt.path); got != tt.want {
				t.Errorf("redactPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMaskTokenKeepsNothingUsable(t *testing.T) {
	const token = "aB3cD4eF12"

	masked := maskToken(token)

	if masked == token {
		t.Fatal("token was not masked")
	}
	if len(masked) != len(token) {
		t.Errorf("masked length = %d, want %d", len(masked), len(token))
	}
	if strings.Contains(masked, token[2:]) {
		t.Error("masked token still contains the secret tail")
	}
}

// The access log must never carry a working share link.
func TestLogHandlerRedactsTokens(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := logHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), logger)

	req := httptest.NewRequest("GET", "/aB3cD4eF12/secret.pdf", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if line := buf.String(); strings.Contains(line, "aB3cD4eF12") {
		t.Errorf("access log leaked the share token: %s", line)
	}
}

// `curl https://send.to/` used to panic on a nil template and answer 500.
func TestViewHandlerServesTextUsage(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "*/*")

	w := httptest.NewRecorder()
	srvr.viewHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"UPLOAD", "curl --upload-file", "Max-Downloads", "storage backend"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("usage text is missing %q:\n%s", want, body)
		}
	}
}
