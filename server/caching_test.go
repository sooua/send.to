package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sooua/send.to/server/storage"
)

// countingStorage counts the reads a request costs the backend. On S3 each one
// is a billed API call, so a redundant read is not merely slow.
type countingStorage struct {
	storage.Storage
	gets atomic.Int64
}

func (c *countingStorage) Get(ctx context.Context, token, filename string, rng *storage.Range) (io.ReadCloser, uint64, error) {
	c.gets.Add(1)
	return c.Storage.Get(ctx, token, filename, rng)
}

// cachingTestServer builds a server over local storage with the given options,
// and returns the read counter alongside it.
func cachingTestServer(t *testing.T, options ...OptionFn) (*Server, *countingStorage) {
	t.Helper()

	tmpDir := t.TempDir()

	local, err := storage.NewLocalStorage(tmpDir, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingStorage{Storage: local}

	options = append([]OptionFn{
		UseStorage(counting),
		RandomTokenLength(10),
		TempPath(os.TempDir() + "/"),
		Logger(slog.New(slog.NewJSONHandler(io.Discard, nil))),
	}, options...)

	srvr, err := New(options...)
	if err != nil {
		t.Fatal(err)
	}

	return srvr, counting
}

// uploadForCaching stores one file through the real PUT path and returns its
// token.
func uploadForCaching(t *testing.T, srvr *Server, filename, content string, headers map[string]string) string {
	t.Helper()

	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(content))
	req.ContentLength = int64(len(content))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("upload returned %d: %s", w.Code, w.Body.String())
	}

	parts := strings.Split(strings.TrimSpace(w.Body.String()), "/")
	if len(parts) < 2 {
		t.Fatalf("unexpected upload URL %q", w.Body.String())
	}

	return parts[len(parts)-2]
}

func download(srvr *Server, token, filename string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/"+token+"/"+filename, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	req = mux.SetURLVars(req, map[string]string{"token": token, "filename": filename})

	w := httptest.NewRecorder()
	srvr.getHandler(w, req)

	return w
}

func TestDownloadIsNoStoreByDefault(t *testing.T) {
	srvr, _ := cachingTestServer(t)

	token := uploadForCaching(t, srvr, "a.txt", "hello", nil)
	resp := download(srvr, token, "a.txt", nil)

	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — caching must stay opt-in", got)
	}
	if resp.Header().Get("ETag") == "" {
		t.Error("no ETag, so a client cannot revalidate even when it already holds the file")
	}
}

func TestDownloadIsCacheableWhenConfigured(t *testing.T) {
	srvr, _ := cachingTestServer(t, CacheMaxAge(600))

	token := uploadForCaching(t, srvr, "a.txt", "hello", nil)
	resp := download(srvr, token, "a.txt", nil)

	if got := resp.Header().Get("Cache-Control"); got != "public, max-age=600, immutable" {
		t.Errorf("Cache-Control = %q, want public, max-age=600, immutable", got)
	}
}

func TestDownloadWithDownloadLimitIsNeverCacheable(t *testing.T) {
	srvr, _ := cachingTestServer(t, CacheMaxAge(600))

	token := uploadForCaching(t, srvr, "a.txt", "hello", map[string]string{"Max-Downloads": "3"})
	resp := download(srvr, token, "a.txt", nil)

	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — a cached copy is served without the origin counting it, so Max-Downloads would not be honoured", got)
	}
}

func TestEncryptedDownloadIsNeverCacheable(t *testing.T) {
	srvr, _ := cachingTestServer(t, CacheMaxAge(600))

	token := uploadForCaching(t, srvr, "a.txt", "hello", map[string]string{"X-Encrypt-Password": "hunter2"})
	resp := download(srvr, token, "a.txt", map[string]string{"X-Decrypt-Password": "hunter2"})

	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — the response depends on a password header, and a cache that gets that wrong serves one visitor's plaintext to another", got)
	}
}

func TestCacheLifetimeNeverOutlivesTheLink(t *testing.T) {
	// Max-Days: 1 is far shorter than the configured 30 days, so the shorter
	// one has to win: a cached copy that survives Max-Days breaks the promise
	// the uploader was given.
	srvr, _ := cachingTestServer(t, CacheMaxAge(30*24*3600))

	token := uploadForCaching(t, srvr, "a.txt", "hello", map[string]string{"Max-Days": "1"})
	resp := download(srvr, token, "a.txt", nil)

	control := resp.Header().Get("Cache-Control")

	age, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(control, "public, max-age="), ", immutable"))
	if err != nil {
		t.Fatalf("could not read max-age from %q: %v", control, err)
	}
	if age > 24*3600 {
		t.Errorf("max-age = %d, longer than the file's own Max-Days of one day", age)
	}
}

func TestConditionalRequestSkipsTheStorageRead(t *testing.T) {
	srvr, counting := cachingTestServer(t, CacheMaxAge(600))

	token := uploadForCaching(t, srvr, "a.txt", "hello", nil)

	first := download(srvr, token, "a.txt", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag to revalidate with")
	}

	counting.gets.Store(0)

	second := download(srvr, token, "a.txt", map[string]string{"If-None-Match": etag})

	if second.Code != http.StatusNotModified {
		t.Fatalf("revalidation returned %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried %d bytes of body", second.Body.Len())
	}
	if got := counting.gets.Load(); got != 1 {
		t.Errorf("revalidation cost %d storage reads, want 1 (the metadata check) — the payload must not be fetched", got)
	}
}

func TestUnconditionalDownloadCostsTwoStorageReads(t *testing.T) {
	srvr, counting := cachingTestServer(t)

	token := uploadForCaching(t, srvr, "a.txt", "hello", nil)

	counting.gets.Store(0)
	resp := download(srvr, token, "a.txt", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("download returned %d", resp.Code)
	}

	// Metadata, then the payload. There used to be a third: increaseDownload
	// re-read the metadata after every download to discover the upload had no
	// download limit and that there was nothing to record.
	if got := counting.gets.Load(); got != 2 {
		t.Errorf("download cost %d storage reads, want 2 (metadata + payload)", got)
	}
}

func TestDownloadLimitStillCountsAfterTheRoundTripSaving(t *testing.T) {
	srvr, _ := cachingTestServer(t)

	token := uploadForCaching(t, srvr, "a.txt", "hello", map[string]string{"Max-Downloads": "2"})

	for i := 1; i <= 2; i++ {
		if resp := download(srvr, token, "a.txt", nil); resp.Code != http.StatusOK {
			t.Fatalf("download %d returned %d", i, resp.Code)
		}
	}

	if resp := download(srvr, token, "a.txt", nil); resp.Code != http.StatusNotFound {
		t.Errorf("third download returned %d, want 404 — the Max-Downloads budget was not being counted", resp.Code)
	}
}

func TestMissingFileIsNotCacheable(t *testing.T) {
	srvr, _ := cachingTestServer(t, CacheMaxAge(600))

	resp := download(srvr, "nosuchtoken", "a.txt", nil)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("returned %d, want 404", resp.Code)
	}
	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control on a 404 = %q, want no-store — otherwise a cache keeps the error in place of the file", got)
	}
}

func TestHeadCarriesTheSameValidator(t *testing.T) {
	srvr, _ := cachingTestServer(t, CacheMaxAge(600))

	token := uploadForCaching(t, srvr, "a.txt", "hello", nil)

	getResp := download(srvr, token, "a.txt", nil)

	req := httptest.NewRequest("HEAD", "/"+token+"/a.txt", nil)
	req = mux.SetURLVars(req, map[string]string{"token": token, "filename": "a.txt"})
	headResp := httptest.NewRecorder()
	srvr.headHandler(headResp, req)

	if got, want := headResp.Header().Get("ETag"), getResp.Header().Get("ETag"); got != want {
		t.Errorf("HEAD ETag %q does not match GET ETag %q, so revalidation through HEAD never matches", got, want)
	}
}

func TestETagMatching(t *testing.T) {
	const etag = `"abc"`

	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{`"abc"`, true},
		{`W/"abc"`, true},
		{`"other", "abc"`, true},
		{`"other"`, false},
		{"*", true},
		{`"ab"`, false},
	}

	for _, tc := range cases {
		if got := etagMatches(tc.header, etag); got != tc.want {
			t.Errorf("etagMatches(%q, %q) = %v, want %v", tc.header, etag, got, tc.want)
		}
	}
}
