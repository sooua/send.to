package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sooua/send.to/server/storage"
)

// newTestServer creates a Server backed by local storage in a temp directory.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "sendto-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store, err := storage.NewLocalStorage(tmpDir, slog.Default())
	if err != nil {
		t.Fatalf("failed to create local storage: %v", err)
	}

	srvr, err := New(
		UseStorage(store),
		RandomTokenLength(10),
		TempPath(os.TempDir()+"/"),
		Logger(slog.New(slog.NewJSONHandler(io.Discard, nil))),
	)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	return srvr, tmpDir
}

func TestPutAndGetHandler(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	fileContent := "Hello, send.to!"
	filename := "test.txt"

	// PUT upload
	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT returned status %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	uploadURL := strings.TrimSpace(string(body))

	// Extract token from URL (last two path segments are token/filename)
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	if len(parts) < 2 {
		t.Fatalf("unexpected upload URL format: %s", uploadURL)
	}
	tok := parts[len(parts)-2]

	// Verify deletion URL header is set
	deleteURL := resp.Header.Get("X-Url-Delete")
	if deleteURL == "" {
		t.Error("X-Url-Delete header not set")
	}

	// GET download
	req2 := httptest.NewRequest("GET", "/"+tok+"/"+filename, nil)
	req2 = mux.SetURLVars(req2, map[string]string{
		"token":    tok,
		"filename": filename,
		"action":   "",
	})

	w2 := httptest.NewRecorder()
	srvr.getHandler(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		body2, _ := io.ReadAll(resp2.Body)
		t.Fatalf("GET returned status %d: %s", resp2.StatusCode, string(body2))
	}

	downloaded, _ := io.ReadAll(resp2.Body)
	if string(downloaded) != fileContent {
		t.Errorf("downloaded content = %q, want %q", string(downloaded), fileContent)
	}

	// Verify response headers
	if resp2.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", resp2.Header.Get("Content-Type"))
	}
	if resp2.Header.Get("Content-Disposition") == "" {
		t.Error("Content-Disposition header not set")
	}
}

func TestPutEmptyFile(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	req := httptest.NewRequest("PUT", "/empty.txt", strings.NewReader(""))
	req.ContentLength = 0
	req = mux.SetURLVars(req, map[string]string{"filename": "empty.txt"})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty file, got %d", resp.StatusCode)
	}
}

func TestPutMaxUploadSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sendto-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, _ := storage.NewLocalStorage(tmpDir, slog.Default())
	srvr, _ := New(
		UseStorage(store),
		RandomTokenLength(10),
		TempPath(os.TempDir()+"/"),
		MaxUploadSize(1), // 1KB limit
		Logger(slog.New(slog.NewJSONHandler(io.Discard, nil))),
	)

	// Upload 2KB file (exceeds 1KB limit)
	bigContent := strings.Repeat("A", 2048)
	req := httptest.NewRequest("PUT", "/big.txt", strings.NewReader(bigContent))
	req.ContentLength = int64(len(bigContent))
	req = mux.SetURLVars(req, map[string]string{"filename": "big.txt"})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 for oversized file, got %d", resp.StatusCode)
	}
}

func TestHeadHandler(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	fileContent := "head test content"
	filename := "head.txt"

	// Upload first
	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	// HEAD request
	req2 := httptest.NewRequest("HEAD", "/"+tok+"/"+filename, nil)
	req2 = mux.SetURLVars(req2, map[string]string{
		"token":    tok,
		"filename": filename,
	})

	w2 := httptest.NewRecorder()
	srvr.headHandler(w2, req2)

	resp := w2.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD returned status %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Length") == "" {
		t.Error("Content-Length header not set")
	}
	if resp.Header.Get("Content-Type") == "" {
		t.Error("Content-Type header not set")
	}
	if resp.Header.Get("X-Remaining-Downloads") == "" {
		t.Error("X-Remaining-Downloads header not set")
	}
	if resp.Header.Get("X-Remaining-Days") == "" {
		t.Error("X-Remaining-Days header not set")
	}
}

func TestDeleteHandler(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	fileContent := "delete me"
	filename := "delete.txt"

	// Upload
	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	resp := w.Result()
	deleteURL := resp.Header.Get("X-Url-Delete")
	// Extract token and deletion token from delete URL
	body, _ := io.ReadAll(resp.Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	deleteParts := strings.Split(strings.TrimPrefix(deleteURL, "http://"), "/")
	deletionToken := deleteParts[len(deleteParts)-1]

	// Delete
	req2 := httptest.NewRequest("DELETE", "/"+tok+"/"+filename+"/"+deletionToken, nil)
	req2 = mux.SetURLVars(req2, map[string]string{
		"token":         tok,
		"filename":      filename,
		"deletionToken": deletionToken,
	})

	w2 := httptest.NewRecorder()
	srvr.deleteHandler(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		body2, _ := io.ReadAll(resp2.Body)
		t.Fatalf("DELETE returned status %d: %s", resp2.StatusCode, string(body2))
	}

	// Verify file is gone
	req3 := httptest.NewRequest("GET", "/"+tok+"/"+filename, nil)
	req3 = mux.SetURLVars(req3, map[string]string{
		"token":    tok,
		"filename": filename,
	})

	w3 := httptest.NewRecorder()
	srvr.getHandler(w3, req3)

	if w3.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", w3.Result().StatusCode)
	}
}

func TestDeleteHandlerWrongToken(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	fileContent := "protected file"
	filename := "protected.txt"

	// Upload
	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	// Try to delete with wrong token
	req2 := httptest.NewRequest("DELETE", "/"+tok+"/"+filename+"/wrongtoken", nil)
	req2 = mux.SetURLVars(req2, map[string]string{
		"token":         tok,
		"filename":      filename,
		"deletionToken": "wrongtoken",
	})

	w2 := httptest.NewRecorder()
	srvr.deleteHandler(w2, req2)

	if w2.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for wrong deletion token, got %d", w2.Result().StatusCode)
	}
}

func TestCheckMetadataMaxDownloads(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	filename := "limited.txt"
	fileContent := "limited downloads"

	// Upload with Max-Downloads: 2
	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req.Header.Set("Max-Downloads", "2")
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	ctx := context.Background()

	// First download
	if _, err := srvr.checkMetadata(ctx, tok, filename); err != nil {
		t.Fatalf("first download rejected: %v", err)
	}
	if err := srvr.increaseDownload(ctx, tok, filename); err != nil {
		t.Fatalf("recording first download failed: %v", err)
	}

	meta, err := srvr.checkMetadata(ctx, tok, filename)
	if err != nil {
		t.Fatalf("second download rejected: %v", err)
	}
	if meta.Downloads != 1 {
		t.Errorf("downloads = %d, want 1", meta.Downloads)
	}

	// Second download exhausts the budget, which also deletes the upload.
	if err := srvr.increaseDownload(ctx, tok, filename); err != nil {
		t.Fatalf("recording second download failed: %v", err)
	}

	// Third download should fail
	if _, err := srvr.checkMetadata(ctx, tok, filename); err == nil {
		t.Error("expected error for exceeded max downloads")
	}

	// The blob must not survive its own expiry.
	if _, err := srvr.storage.Head(ctx, tok, filename); err == nil {
		t.Error("expected exhausted upload to be deleted from storage")
	}
}

// A Range request is one slice of a resumed transfer, so it must not consume
// a download from the Max-Downloads budget.
func TestGetHandlerRangeDoesNotCountAsDownload(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	filename := "ranged.txt"
	fileContent := "0123456789abcdefghij"

	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req.Header.Set("Max-Downloads", "1")
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	// Two partial reads of the same file.
	for i := 0; i < 2; i++ {
		getReq := httptest.NewRequest("GET", "/"+tok+"/"+filename, nil)
		getReq.Header.Set("Range", "bytes=0-4")
		getReq = mux.SetURLVars(getReq, map[string]string{"token": tok, "filename": filename})

		getW := httptest.NewRecorder()
		srvr.getHandler(getW, getReq)

		if code := getW.Result().StatusCode; code != http.StatusPartialContent {
			t.Fatalf("range request %d returned %d, want 206", i, code)
		}
	}

	meta, err := srvr.checkMetadata(context.Background(), tok, filename)
	if err != nil {
		t.Fatalf("upload should still be available after range requests: %v", err)
	}
	if meta.Downloads != 0 {
		t.Errorf("downloads = %d after range requests, want 0", meta.Downloads)
	}

	// A full download does count, and exhausts the single-download budget.
	getReq := httptest.NewRequest("GET", "/"+tok+"/"+filename, nil)
	getReq = mux.SetURLVars(getReq, map[string]string{"token": tok, "filename": filename})
	srvr.getHandler(httptest.NewRecorder(), getReq)

	if _, err := srvr.checkMetadata(context.Background(), tok, filename); err == nil {
		t.Error("expected upload to be exhausted after one full download")
	}
}

// A download that fails partway through must not consume the budget.
func TestGetHandlerFailedTransferDoesNotCountAsDownload(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	filename := "fragile.txt"
	fileContent := strings.Repeat("x", 4096)

	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req.Header.Set("Max-Downloads", "1")
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	getReq := httptest.NewRequest("GET", "/"+tok+"/"+filename, nil)
	getReq = mux.SetURLVars(getReq, map[string]string{"token": tok, "filename": filename})

	srvr.getHandler(&failingResponseWriter{header: http.Header{}}, getReq)

	meta, err := srvr.checkMetadata(context.Background(), tok, filename)
	if err != nil {
		t.Fatalf("upload should survive a failed transfer: %v", err)
	}
	if meta.Downloads != 0 {
		t.Errorf("downloads = %d after failed transfer, want 0", meta.Downloads)
	}
}

// failingResponseWriter simulates a client that disconnects mid-body.
type failingResponseWriter struct {
	header http.Header
}

func (f *failingResponseWriter) Header() http.Header { return f.header }
func (f *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}
func (f *failingResponseWriter) WriteHeader(int) {}

func TestPostHandler(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "multipart.txt")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	fileContent := "multipart upload content"
	_, _ = part.Write([]byte(fileContent))
	writer.Close()

	req := httptest.NewRequest("POST", "/", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	w := httptest.NewRecorder()
	srvr.postHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST returned status %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	uploadURL := strings.TrimSpace(string(body))
	if uploadURL == "" {
		t.Fatal("POST returned empty URL")
	}

	// Extract token and download
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]
	fname := parts[len(parts)-1]

	req2 := httptest.NewRequest("GET", "/"+tok+"/"+fname, nil)
	req2 = mux.SetURLVars(req2, map[string]string{
		"token":    tok,
		"filename": fname,
	})

	w2 := httptest.NewRecorder()
	srvr.getHandler(w2, req2)

	downloaded, _ := io.ReadAll(w2.Result().Body)
	if string(downloaded) != fileContent {
		t.Errorf("downloaded = %q, want %q", string(downloaded), fileContent)
	}
}

func TestPutWithEncryption(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	fileContent := "encrypted content for testing"
	filename := "encrypted.txt"
	password := "my-secret-password"

	// Upload with encryption
	req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(fileContent))
	req.ContentLength = int64(len(fileContent))
	req.Header.Set("X-Encrypt-Password", password)
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT returned status %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	uploadURL := strings.TrimSpace(string(body))
	parts := strings.Split(strings.TrimPrefix(uploadURL, "http://"), "/")
	tok := parts[len(parts)-2]

	// Verify metadata shows encrypted
	ctx := context.Background()
	meta, err := srvr.checkMetadata(ctx, tok, filename)
	if err != nil {
		t.Fatalf("checkMetadata failed: %v", err)
	}
	if !meta.Encrypted {
		t.Error("metadata should show file as encrypted")
	}

	// Download with correct password
	req2 := httptest.NewRequest("GET", "/"+tok+"/"+filename, nil)
	req2.Header.Set("X-Decrypt-Password", password)
	req2 = mux.SetURLVars(req2, map[string]string{
		"token":    tok,
		"filename": filename,
	})

	w2 := httptest.NewRecorder()
	srvr.getHandler(w2, req2)

	resp2 := w2.Result()
	if resp2.StatusCode != http.StatusOK {
		body2, _ := io.ReadAll(resp2.Body)
		t.Fatalf("GET returned status %d: %s", resp2.StatusCode, string(body2))
	}

	downloaded, _ := io.ReadAll(resp2.Body)
	if string(downloaded) != fileContent {
		t.Errorf("decrypted content = %q, want %q", string(downloaded), fileContent)
	}
}

func TestGetNonExistentFile(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	req := httptest.NewRequest("GET", "/nonexistent/file.txt", nil)
	req = mux.SetURLVars(req, map[string]string{
		"token":    "nonexistent",
		"filename": "file.txt",
	})

	w := httptest.NewRecorder()
	srvr.getHandler(w, req)

	if w.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Result().StatusCode)
	}
}

func TestMetadataForRequest(t *testing.T) {
	t.Run("default metadata", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test.txt", nil)
		meta, err := metadataForRequest("text/plain", 100, 10, req)
		if err != nil {
			t.Fatalf("metadataForRequest failed: %v", err)
		}

		if meta.ContentType != "text/plain" {
			t.Errorf("ContentType = %q, want \"text/plain\"", meta.ContentType)
		}
		if meta.ContentLength != 100 {
			t.Errorf("ContentLength = %d, want 100", meta.ContentLength)
		}
		if meta.MaxDownloads != -1 {
			t.Errorf("MaxDownloads = %d, want -1", meta.MaxDownloads)
		}
		if !meta.MaxDate.IsZero() {
			t.Error("MaxDate should be zero")
		}
		if meta.Downloads != 0 {
			t.Errorf("Downloads = %d, want 0", meta.Downloads)
		}
		if len(meta.DeletionToken) != 20 {
			t.Errorf("DeletionToken length = %d, want 20", len(meta.DeletionToken))
		}
		if meta.Encrypted {
			t.Error("should not be encrypted")
		}
	})

	t.Run("with max downloads", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test.txt", nil)
		req.Header.Set("Max-Downloads", "5")
		meta, err := metadataForRequest("text/plain", 100, 10, req)
		if err != nil {
			t.Fatalf("metadataForRequest failed: %v", err)
		}

		if meta.MaxDownloads != 5 {
			t.Errorf("MaxDownloads = %d, want 5", meta.MaxDownloads)
		}
	})

	t.Run("with max days", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test.txt", nil)
		req.Header.Set("Max-Days", "7")
		meta, err := metadataForRequest("text/plain", 100, 10, req)
		if err != nil {
			t.Fatalf("metadataForRequest failed: %v", err)
		}

		if meta.MaxDate.IsZero() {
			t.Error("MaxDate should not be zero")
		}
	})

	t.Run("with encryption", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test.txt", nil)
		req.Header.Set("X-Encrypt-Password", "secret")
		meta, err := metadataForRequest("application/pdf", 100, 10, req)
		if err != nil {
			t.Fatalf("metadataForRequest failed: %v", err)
		}

		if !meta.Encrypted {
			t.Error("should be encrypted")
		}
		if meta.ContentType != "text/plain; charset=utf-8" {
			t.Errorf("ContentType = %q, want \"text/plain; charset=utf-8\"", meta.ContentType)
		}
		if meta.DecryptedContentType != "application/pdf" {
			t.Errorf("DecryptedContentType = %q, want \"application/pdf\"", meta.DecryptedContentType)
		}
	})
}

func TestCheckMetadataJSON(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Manually store metadata and file
	tok := "testtoken"
	filename := "manual.txt"
	ctx := context.Background()

	meta := metadata{
		ContentType:   "text/plain",
		ContentLength: 5,
		Downloads:     0,
		MaxDownloads:  -1,
		DeletionToken: "deltoken",
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(meta); err != nil {
		t.Fatalf("failed to encode metadata: %v", err)
	}

	if err := srvr.storage.Put(ctx, tok, fmt.Sprintf("%s.metadata", filename), &buf, "text/json", uint64(buf.Len())); err != nil {
		t.Fatalf("failed to store metadata: %v", err)
	}

	if err := srvr.storage.Put(ctx, tok, filename, strings.NewReader("hello"), "text/plain", 5); err != nil {
		t.Fatalf("failed to store file: %v", err)
	}

	// Check metadata
	result, err := srvr.checkMetadata(ctx, tok, filename)
	if err != nil {
		t.Fatalf("checkMetadata failed: %v", err)
	}

	if result.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want \"text/plain\"", result.ContentType)
	}
	if result.DeletionToken != "deltoken" {
		t.Errorf("DeletionToken = %q, want \"deltoken\"", result.DeletionToken)
	}
}
