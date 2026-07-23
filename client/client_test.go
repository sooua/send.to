package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadSendsLimitsAndParsesResult(t *testing.T) {
	var gotDays, gotDownloads, gotPassword, gotAccept, gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDays = r.Header.Get("Max-Days")
		gotDownloads = r.Header.Get("Max-Downloads")
		gotPassword = r.Header.Get("X-Encrypt-Password")
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")

		expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
		maxDownloads := 3

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Result{
			URL:          "https://send.to/tok/report.pdf",
			DeleteURL:    "https://send.to/tok/report.pdf/del",
			Filename:     "report.pdf",
			Size:         11,
			Encrypted:    true,
			MaxDownloads: &maxDownloads,
			ExpiresAt:    &expires,
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.Username, c.Password = "user", "pass"

	result, err := c.Upload(context.Background(), "report.pdf", strings.NewReader("hello world"), 11, UploadOptions{
		Days:         7,
		MaxDownloads: 3,
		Password:     "s3cret",
	})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if gotDays != "7" {
		t.Errorf("Max-Days = %q, want \"7\"", gotDays)
	}
	if gotDownloads != "3" {
		t.Errorf("Max-Downloads = %q, want \"3\"", gotDownloads)
	}
	if gotPassword != "s3cret" {
		t.Errorf("X-Encrypt-Password = %q", gotPassword)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if gotAuth == "" {
		t.Error("basic auth was not sent")
	}

	if result.URL != "https://send.to/tok/report.pdf" {
		t.Errorf("URL = %q", result.URL)
	}
	if result.MaxDownloads == nil || *result.MaxDownloads != 3 {
		t.Error("MaxDownloads was not parsed")
	}
	if result.ExpiresAt == nil {
		t.Error("ExpiresAt was not parsed")
	}
}

// An older server answers a bare URL, which decodes into an empty struct
// rather than erroring — that must not look like success.
func TestUploadRejectsNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "https://send.to/tok/a.txt")
	}))
	defer srv.Close()

	_, err := New(srv.URL).Upload(context.Background(), "a.txt", strings.NewReader("x"), 1, UploadOptions{})
	if err == nil {
		t.Fatal("expected an error for a non-JSON response")
	}
}

func TestUploadSurfacesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Max-Downloads must be a positive integer", http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := New(srv.URL).Upload(context.Background(), "a.txt", strings.NewReader("x"), 1, UploadOptions{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "Max-Downloads must be a positive integer") {
		t.Errorf("error did not include the server's message: %v", err)
	}
}

func TestDownloadResumesFromOffset(t *testing.T) {
	const body = "0123456789"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng != "bytes=4-" {
			t.Errorf("Range = %q, want bytes=4-", rng)
		}
		w.Header().Set("Content-Range", "bytes 4-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, body[4:])
	}))
	defer srv.Close()

	var out strings.Builder
	written, resumed, err := New(srv.URL).Download(context.Background(), srv.URL+"/tok/a.txt", &out, DownloadOptions{Offset: 4})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}

	if !resumed {
		t.Error("expected the transfer to be reported as resumed")
	}
	if out.String() != body[4:] {
		t.Errorf("body = %q, want %q", out.String(), body[4:])
	}
	if written != int64(len(body[4:])) {
		t.Errorf("written = %d, want %d", written, len(body[4:]))
	}
}

// A server that ignores Range answers 200 with the whole file; the caller has
// to know so it can discard what it already had.
func TestDownloadReportsIgnoredResume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "0123456789")
	}))
	defer srv.Close()

	var out strings.Builder
	_, resumed, err := New(srv.URL).Download(context.Background(), srv.URL+"/tok/a.txt", &out, DownloadOptions{Offset: 4})
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if resumed {
		t.Error("a 200 response must not be reported as resumed")
	}
}

func TestStatDoesNotSpendADownload(t *testing.T) {
	var method string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "42")
		w.Header().Set("X-Remaining-Downloads", "2")
		w.Header().Set("X-Remaining-Days", "5")
		w.Header().Set("Accept-Ranges", "bytes")
	}))
	defer srv.Close()

	info, err := New(srv.URL).Stat(context.Background(), srv.URL+"/tok/notes.md")
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	if method != http.MethodHead {
		t.Errorf("method = %s, want HEAD", method)
	}
	if info.Filename != "notes.md" {
		t.Errorf("Filename = %q", info.Filename)
	}
	if info.Size != 42 || info.RemainingDownloads != "2" || info.RemainingDays != "5" || !info.SupportsRange {
		t.Errorf("info not parsed: %+v", info)
	}
}

// Deleting something already gone is the desired end state, not an error.
func TestDeleteTreats404AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	if err := New(srv.URL).Delete(context.Background(), srv.URL+"/tok/a.txt/del"); err != nil {
		t.Errorf("delete returned %v, want nil", err)
	}
}

func TestConfigResolvePrecedence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENDTO_CONFIG_DIR", dir)

	cfg := &Config{
		Default: "home",
		Profiles: map[string]Profile{
			"home": {URL: "https://home.example", Username: "h"},
			"work": {URL: "https://work.example", Username: "w"},
		},
	}

	t.Run("explicit url wins", func(t *testing.T) {
		p, err := cfg.Resolve("https://flag.example", "work")
		if err != nil {
			t.Fatal(err)
		}
		if p.URL != "https://flag.example" {
			t.Errorf("URL = %q", p.URL)
		}
	})

	t.Run("profile beats default", func(t *testing.T) {
		p, err := cfg.Resolve("", "work")
		if err != nil {
			t.Fatal(err)
		}
		if p.URL != "https://work.example" || p.Username != "w" {
			t.Errorf("profile = %+v", p)
		}
	})

	t.Run("default is used", func(t *testing.T) {
		p, err := cfg.Resolve("", "")
		if err != nil {
			t.Fatal(err)
		}
		if p.URL != "https://home.example" {
			t.Errorf("URL = %q", p.URL)
		}
	})

	t.Run("env overrides credentials", func(t *testing.T) {
		t.Setenv("SENDTO_USER", "envuser")
		t.Setenv("SENDTO_PASS", "envpass")

		p, err := cfg.Resolve("", "work")
		if err != nil {
			t.Fatal(err)
		}
		if p.Username != "envuser" || p.Password != "envpass" {
			t.Errorf("credentials = %+v", p)
		}
	})

	t.Run("bare host gets https", func(t *testing.T) {
		p, err := cfg.Resolve("files.example.com", "")
		if err != nil {
			t.Fatal(err)
		}
		if p.URL != "https://files.example.com" {
			t.Errorf("URL = %q, want https:// prefix", p.URL)
		}
	})

	t.Run("unknown profile is an error", func(t *testing.T) {
		if _, err := cfg.Resolve("", "nope"); err == nil {
			t.Error("expected an error for an unknown profile")
		}
	})

	t.Run("nothing configured is an actionable error", func(t *testing.T) {
		empty := &Config{Profiles: map[string]Profile{}}
		_, err := empty.Resolve("", "")
		if err == nil || !strings.Contains(err.Error(), "send config add") {
			t.Errorf("error should suggest the fix, got %v", err)
		}
	})
}

func TestConfigRoundTripIsOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENDTO_CONFIG_DIR", dir)

	cfg := &Config{Default: "a", Profiles: map[string]Profile{"a": {URL: "https://a.example", Password: "hunter2"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Profiles["a"].URL != "https://a.example" {
		t.Errorf("round trip lost data: %+v", loaded)
	}

	// The file can hold a server password.
	if info, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		if perm := info.Mode().Perm(); perm&0077 != 0 && os.Getenv("GOOS") != "windows" {
			t.Errorf("config.json mode = %o, want no group/other access", perm)
		}
	}
}

func TestHistoryAddFindRemovePrune(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENDTO_CONFIG_DIR", dir)

	h := &History{}
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	h.Add("https://send.to", &Result{URL: "https://send.to/a/one.txt", DeleteURL: "https://send.to/a/one.txt/d", ExpiresAt: &future})
	h.Add("https://send.to", &Result{URL: "https://send.to/b/two.txt", ExpiresAt: &past})

	if got := h.Find("https://send.to/a/one.txt"); got == nil || got.DeleteURL == "" {
		t.Fatal("Find did not return the entry with its delete URL")
	}
	if h.Find("https://send.to/missing") != nil {
		t.Error("Find returned an entry for an unknown URL")
	}

	if removed := h.Prune(); removed != 1 {
		t.Errorf("Prune removed %d, want 1", removed)
	}
	if len(h.Entries) != 1 {
		t.Fatalf("entries left = %d, want 1", len(h.Entries))
	}

	if err := h.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].URL != "https://send.to/a/one.txt" {
		t.Errorf("history round trip failed: %+v", loaded.Entries)
	}

	if !loaded.Remove("https://send.to/a/one.txt") {
		t.Error("Remove reported nothing removed")
	}
	if len(loaded.Entries) != 0 {
		t.Errorf("entries left = %d, want 0", len(loaded.Entries))
	}
}

// A corrupt history file must not stop an upload from being recorded.
func TestLoadHistoryToleratesCorruption(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SENDTO_CONFIG_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, "history.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	h, err := LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory returned %v, want a usable empty history", err)
	}
	if len(h.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(h.Entries))
	}
}
