package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := securityHeadersHandler(inner)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "geolocation=(), camera=(), microphone=()"},
	}

	for _, tt := range tests {
		got := resp.Header.Get(tt.header)
		if got != tt.expected {
			t.Errorf("header %s = %q, want %q", tt.header, got, tt.expected)
		}
	}

	// CSP should be present
	csp := resp.Header.Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header not set")
	}

	// HSTS should NOT be set for plain HTTP
	hsts := resp.Header.Get("Strict-Transport-Security")
	if hsts != "" {
		t.Errorf("HSTS should not be set for plain HTTP, got %q", hsts)
	}
}

func TestSecurityHeadersHSTSWithTLS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := securityHeadersHandler(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()

	hsts := resp.Header.Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("HSTS should be set when X-Forwarded-Proto is https")
	}
	if hsts != "max-age=31536000; includeSubDomains" {
		t.Errorf("HSTS = %q, want \"max-age=31536000; includeSubDomains\"", hsts)
	}
}

func TestLoveHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := LoveHandler(inner)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()

	if resp.Header.Get("x-made-with") != "<3 by sooua" {
		t.Errorf("x-made-with = %q", resp.Header.Get("x-made-with"))
	}
	if resp.Header.Get("x-served-by") != "Proudly served by send.to" {
		t.Errorf("x-served-by = %q", resp.Header.Get("x-served-by"))
	}
	if resp.Header.Get("server") != "send.to HTTP Server" {
		t.Errorf("server = %q", resp.Header.Get("server"))
	}
}

func TestHealthHandler(t *testing.T) {
	srvr, _ := New()

	req := httptest.NewRequest("GET", "/health.html", nil)
	w := httptest.NewRecorder()
	srvr.healthHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health handler returned %d", resp.StatusCode)
	}
}

func TestHealthHandlerJSON(t *testing.T) {
	srvr, _ := New()

	req := httptest.NewRequest("GET", "/health", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	srvr.healthHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health handler returned %d", resp.StatusCode)
	}

	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Uptime  string `json:"uptime"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("health response is not JSON: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want \"ok\"", body.Status)
	}
	if body.Version == "" {
		t.Error("version should not be empty")
	}
}

func TestBasicAuthHandlerNoAuth(t *testing.T) {
	srvr, _ := New()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srvr.basicAuthHandler(inner)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// No auth configured, should pass through
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 with no auth configured, got %d", w.Result().StatusCode)
	}
}

func TestBasicAuthHandlerWithCredentials(t *testing.T) {
	srvr, _ := New(HTTPAuthCredentials("admin", "secret"))

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := srvr.basicAuthHandler(inner)

	t.Run("no credentials", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Result().StatusCode)
		}
	})

	t.Run("wrong credentials", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("admin", "wrong")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", w.Result().StatusCode)
		}
	})

	t.Run("correct credentials", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("admin", "secret")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Result().StatusCode)
		}
	})
}
