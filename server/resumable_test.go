package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// newSessionTestServer keeps spool files inside the test's own directory so a
// failure leaves nothing behind in the system temp folder.
func newSessionTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	srvr, tmpDir := newTestServer(t)
	srvr.tempPath = tmpDir

	return srvr, tmpDir
}

func createSession(t *testing.T, srvr *Server, filename string, total int64, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/upload/"+filename, nil)
	req.Header.Set("Upload-Length", fmt.Sprint(total))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.createUploadSessionHandler(w, req)

	return w
}

// sessionIDFrom pulls the id out of the session URL the server handed back.
func sessionIDFrom(t *testing.T, location string) string {
	t.Helper()

	parts := strings.Split(strings.TrimSuffix(location, "/"), "/")
	if len(parts) < 2 {
		t.Fatalf("unexpected session URL %q", location)
	}

	return parts[len(parts)-2]
}

func patchChunk(t *testing.T, srvr *Server, id, filename string, start int64, chunk []byte, total int64) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/upload/"+id+"/"+filename, bytes.NewReader(chunk))
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+int64(len(chunk))-1, total))
	req = mux.SetURLVars(req, map[string]string{"id": id, "filename": filename})

	w := httptest.NewRecorder()
	srvr.patchUploadSessionHandler(w, req)

	return w
}

func TestResumableUploadRoundTrip(t *testing.T) {
	srvr, tmpDir := newSessionTestServer(t)
	defer os.RemoveAll(tmpDir)

	payload := bytes.Repeat([]byte("send.to resumable payload\n"), 400)
	total := int64(len(payload))
	filename := "artifact.bin"

	created := createSession(t, srvr, filename, total, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create session = %d, want 201: %s", created.Code, created.Body.String())
	}

	location := created.Header().Get("Location")
	if location == "" {
		t.Fatal("no Location header on the created session")
	}
	if created.Header().Get("Upload-Offset") != "0" {
		t.Errorf("Upload-Offset = %q, want 0", created.Header().Get("Upload-Offset"))
	}

	id := sessionIDFrom(t, location)

	// Two partial chunks, then the tail.
	split := total / 3

	first := patchChunk(t, srvr, id, filename, 0, payload[:split], total)
	if first.Code != http.StatusNoContent {
		t.Fatalf("first chunk = %d, want 204: %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Upload-Offset"); got != fmt.Sprint(split) {
		t.Errorf("Upload-Offset after first chunk = %q, want %d", got, split)
	}

	// A HEAD tells a client that lost its own bookkeeping where to continue.
	headReq := httptest.NewRequest(http.MethodHead, "/upload/"+id+"/"+filename, nil)
	headReq = mux.SetURLVars(headReq, map[string]string{"id": id, "filename": filename})
	headRec := httptest.NewRecorder()
	srvr.headUploadSessionHandler(headRec, headReq)

	if got := headRec.Header().Get("Upload-Offset"); got != fmt.Sprint(split) {
		t.Errorf("HEAD Upload-Offset = %q, want %d", got, split)
	}

	// Re-sending an earlier offset must be refused, not silently duplicated.
	stale := patchChunk(t, srvr, id, filename, 0, payload[:split], total)
	if stale.Code != http.StatusConflict {
		t.Errorf("stale chunk = %d, want 409", stale.Code)
	}
	if got := stale.Header().Get("Upload-Offset"); got != fmt.Sprint(split) {
		t.Errorf("conflict Upload-Offset = %q, want %d", got, split)
	}

	if code := patchChunk(t, srvr, id, filename, split, payload[split:2*split], total).Code; code != http.StatusNoContent {
		t.Fatalf("second chunk = %d, want 204", code)
	}

	final := patchChunk(t, srvr, id, filename, 2*split, payload[2*split:], total)
	if final.Code != http.StatusOK {
		t.Fatalf("final chunk = %d, want 200: %s", final.Code, final.Body.String())
	}
	if final.Header().Get("X-Url-Delete") == "" {
		t.Error("no X-Url-Delete on the completed upload")
	}

	uploadURL := strings.TrimSpace(final.Body.String())
	parts := strings.Split(uploadURL, "/")
	uploadToken := parts[len(parts)-2]

	// The stored bytes must be the ones that were sent, in order.
	getReq := httptest.NewRequest(http.MethodGet, "/"+uploadToken+"/"+filename, nil)
	getReq = mux.SetURLVars(getReq, map[string]string{"token": uploadToken, "filename": filename})
	getRec := httptest.NewRecorder()
	srvr.getHandler(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200", getRec.Code)
	}
	if got, _ := io.ReadAll(getRec.Body); !bytes.Equal(got, payload) {
		t.Errorf("stored %d bytes, want %d identical", len(got), len(payload))
	}

	// The spool files are gone once the upload is stored.
	part, sidecar := srvr.sessionPaths(id)
	for _, p := range []string{part, sidecar} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists after completion", p)
		}
	}
}

func TestResumableUploadAppliesOptions(t *testing.T) {
	srvr, tmpDir := newSessionTestServer(t)
	defer os.RemoveAll(tmpDir)

	payload := []byte("limited")
	filename := "limited.txt"

	created := createSession(t, srvr, filename, int64(len(payload)), map[string]string{
		"Max-Downloads": "2",
		"Max-Days":      "3",
		"Accept":        "application/json",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create session = %d: %s", created.Code, created.Body.String())
	}

	var session struct {
		UploadURL string `json:"upload_url"`
		Length    int64  `json:"length"`
	}
	if err := json.NewDecoder(created.Body).Decode(&session); err != nil {
		t.Fatalf("could not decode the session response: %v", err)
	}
	if session.Length != int64(len(payload)) {
		t.Errorf("session length = %d, want %d", session.Length, len(payload))
	}

	id := sessionIDFrom(t, session.UploadURL)

	done := patchChunk(t, srvr, id, filename, 0, payload, int64(len(payload)))
	if done.Code != http.StatusOK {
		t.Fatalf("final chunk = %d: %s", done.Code, done.Body.String())
	}

	uploadURL := strings.TrimSpace(done.Body.String())
	parts := strings.Split(uploadURL, "/")
	uploadToken := parts[len(parts)-2]

	m, err := srvr.checkMetadata(t.Context(), uploadToken, filename)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if m.MaxDownloads != 2 {
		t.Errorf("MaxDownloads = %d, want 2", m.MaxDownloads)
	}
	if m.MaxDate.IsZero() {
		t.Error("Max-Days was not applied")
	}
	if m.Encrypted {
		t.Error("upload marked encrypted without a password")
	}
}

func TestResumableUploadRejections(t *testing.T) {
	srvr, tmpDir := newSessionTestServer(t)
	defer os.RemoveAll(tmpDir)

	t.Run("missing length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/upload/x.bin", nil)
		req = mux.SetURLVars(req, map[string]string{"filename": "x.bin"})
		w := httptest.NewRecorder()
		srvr.createUploadSessionHandler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("server side encryption", func(t *testing.T) {
		// The password cannot be held between chunks without storing it in
		// clear, so it is refused rather than quietly ignored.
		w := createSession(t, srvr, "x.bin", 10, map[string]string{"X-Encrypt-Password": "hunter2"})
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("invalid limit header", func(t *testing.T) {
		w := createSession(t, srvr, "x.bin", 10, map[string]string{"Max-Downloads": "many"})
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("too large", func(t *testing.T) {
		srvr.maxUploadSize = 5
		defer func() { srvr.maxUploadSize = 0 }()

		w := createSession(t, srvr, "x.bin", 4096, nil)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413", w.Code)
		}
	})

	t.Run("total mismatch", func(t *testing.T) {
		created := createSession(t, srvr, "x.bin", 10, nil)
		id := sessionIDFrom(t, created.Header().Get("Location"))

		w := patchChunk(t, srvr, id, "x.bin", 0, []byte("0123456789"), 999)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	})

	t.Run("unknown session", func(t *testing.T) {
		w := patchChunk(t, srvr, "aaaaaaaaaaaaaaaaaaaaaaaa", "x.bin", 0, []byte("x"), 1)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	t.Run("path traversal in the id", func(t *testing.T) {
		w := patchChunk(t, srvr, "../../etc/passwd", "x.bin", 0, []byte("x"), 1)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})
}

func TestParseUploadContentRange(t *testing.T) {
	cases := []struct {
		in                     string
		ok                     bool
		start, end, totalWants int64
	}{
		{"bytes 0-99/100", true, 0, 99, 100},
		{"bytes 100-199/1000", true, 100, 199, 1000},
		{"bytes 0-0/1", true, 0, 0, 1},
		{"bytes 5-4/10", false, 0, 0, 0},
		{"bytes 0-100/100", false, 0, 0, 0},
		{"bytes 0-99", false, 0, 0, 0},
		{"items 0-99/100", false, 0, 0, 0},
		{"", false, 0, 0, 0},
	}

	for _, tc := range cases {
		start, end, total, err := parseUploadContentRange(tc.in)
		if tc.ok != (err == nil) {
			t.Errorf("%q: err = %v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if tc.ok && (start != tc.start || end != tc.end || total != tc.totalWants) {
			t.Errorf("%q = %d-%d/%d, want %d-%d/%d", tc.in, start, end, total, tc.start, tc.end, tc.totalWants)
		}
	}
}
