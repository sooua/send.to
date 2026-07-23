package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sooua/send.to/server/storage"
)

func putBody(t *testing.T, srvr *Server, filename, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/"+filename, strings.NewReader(body))
	req.ContentLength = int64(len(body))
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	return w
}

func TestStorageQuotaRefusesWhenFull(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Room for two of these and no more.
	body := strings.Repeat("x", 100)
	srvr.maxStorageSize = 250
	srvr.quota = &storageQuota{limit: srvr.maxStorageSize}

	if code := putBody(t, srvr, "a.txt", body).Code; code != http.StatusOK {
		t.Fatalf("first upload = %d", code)
	}
	if code := putBody(t, srvr, "b.txt", body).Code; code != http.StatusOK {
		t.Fatalf("second upload = %d", code)
	}

	third := putBody(t, srvr, "c.txt", body)
	if third.Code != http.StatusInsufficientStorage {
		t.Fatalf("third upload = %d, want 507", third.Code)
	}

	// Deleting one has to hand the space back, or the instance is full for
	// good after the first burst.
	first := "a.txt"
	uploadURL := "" // recover the token from the metadata we know exists
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(fmt.Sprintf("%s/%s/%s.metadata", tmpDir, entry.Name(), first)); err == nil {
			uploadURL = entry.Name()
			break
		}
	}
	if uploadURL == "" {
		t.Fatal("could not find the first upload")
	}

	m, err := srvr.checkMetadata(t.Context(), uploadURL, first)
	if err != nil {
		t.Fatal(err)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/x", nil)
	delReq = mux.SetURLVars(delReq, map[string]string{
		"token": uploadURL, "filename": first, "deletionToken": m.DeletionToken,
	})
	srvr.deleteHandler(httptest.NewRecorder(), delReq)

	if code := putBody(t, srvr, "d.txt", body).Code; code != http.StatusOK {
		t.Errorf("upload after a delete = %d, want 200 — the quota did not release the space", code)
	}
}

// A quota is only meaningful if the counter starts from what is already stored.
func TestStorageQuotaSeedsFromTheBackend(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	body := strings.Repeat("x", 500)
	if code := putBody(t, srvr, "existing.txt", body).Code; code != http.StatusOK {
		t.Fatalf("upload = %d", code)
	}

	srvr.maxStorageSize = 600
	if err := srvr.initStorageQuota(); err != nil {
		t.Fatal(err)
	}

	if used := srvr.quota.usage(); used < 500 {
		t.Errorf("seeded usage = %d, want at least the 500 bytes already stored", used)
	}

	if code := putBody(t, srvr, "another.txt", body).Code; code != http.StatusInsufficientStorage {
		t.Errorf("upload = %d, want 507 — the seed was ignored", code)
	}
}

func TestStorageQuotaCountsResumableUploads(t *testing.T) {
	srvr, tmpDir := newSessionTestServer(t)
	defer os.RemoveAll(tmpDir)

	srvr.maxStorageSize = 100
	srvr.quota = &storageQuota{limit: srvr.maxStorageSize}

	// Refused before a byte is transferred: the declared total cannot fit.
	if code := createSession(t, srvr, "big.bin", 200, nil).Code; code != http.StatusInsufficientStorage {
		t.Errorf("oversized session = %d, want 507", code)
	}

	payload := bytes.Repeat([]byte("y"), 80)
	created := createSession(t, srvr, "ok.bin", int64(len(payload)), nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("session = %d", created.Code)
	}

	id := sessionIDFrom(t, created.Header().Get("Location"))
	if code := patchChunk(t, srvr, id, "ok.bin", 0, payload, int64(len(payload))).Code; code != http.StatusOK {
		t.Fatalf("chunk = %d", code)
	}

	if used := srvr.quota.usage(); used != int64(len(payload)) {
		t.Errorf("usage after a resumable upload = %d, want %d", used, len(payload))
	}
}

func TestTempQuotaRefusesAnotherSession(t *testing.T) {
	srvr, tmpDir := newSessionTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Sized well above the per-session sidecar, which counts towards the spool
	// budget too: a flood of sessions that never send a byte still costs disk.
	srvr.maxTempSize = 8192

	// The declared total is reserved at creation, so a session that could never
	// fit in the spool space is refused up front.
	if code := createSession(t, srvr, "big.bin", 32768, nil).Code; code != http.StatusInsufficientStorage {
		t.Errorf("oversized session = %d, want 507", code)
	}

	created := createSession(t, srvr, "ok.bin", 6000, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("session = %d: %s", created.Code, created.Body.String())
	}
	id := sessionIDFrom(t, created.Header().Get("Location"))

	if code := patchChunk(t, srvr, id, "ok.bin", 0, bytes.Repeat([]byte("z"), 5000), 6000).Code; code != http.StatusNoContent {
		t.Fatalf("first chunk = %d", code)
	}

	if used := srvr.tempUsage(); used < 5000 {
		t.Fatalf("temp usage = %d, want at least the 5000 bytes spooled", used)
	}

	// A second session of the same size no longer fits alongside it.
	if code := createSession(t, srvr, "second.bin", 6000, nil).Code; code != http.StatusInsufficientStorage {
		t.Errorf("second session = %d, want 507", code)
	}

	// The chunk that completes the session already accepted still goes through.
	if code := patchChunk(t, srvr, id, "ok.bin", 5000, bytes.Repeat([]byte("z"), 1000), 6000).Code; code != http.StatusOK {
		t.Errorf("completing chunk = %d, want 200", code)
	}

	// And the spool is empty again once the upload is stored.
	if used := srvr.tempUsage(); used != 0 {
		t.Errorf("temp usage after completion = %d, want 0", used)
	}
}

func TestQuotaCounterNeverGoesNegative(t *testing.T) {
	q := &storageQuota{limit: 1000}

	q.add(100)
	q.sub(400)

	if used := q.usage(); used != 0 {
		t.Errorf("usage = %d, want 0 — drift must not hand out free space", used)
	}

	if !q.allows(1000) {
		t.Error("an empty quota refused a fitting upload")
	}
	if q.allows(1001) {
		t.Error("the quota accepted an upload larger than the limit")
	}

	// A disabled quota allows anything.
	var off *storageQuota
	if !off.allows(1 << 40) {
		t.Error("an unset quota refused an upload")
	}
}

// unsupportedUsageStorage stands in for Google Drive or Storj: everything else
// works, but it cannot say how much it holds.
type unsupportedUsageStorage struct {
	storage.Storage
}

func (unsupportedUsageStorage) Type() string { return "stub" }

func (unsupportedUsageStorage) Usage(context.Context) (uint64, error) {
	return 0, storage.ErrUsageUnsupported
}

// An operator who asked for a limit must never be left believing one is in
// force. On a backend that cannot count, startup fails instead.
func TestStorageQuotaRefusesABackendThatCannotCount(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	srvr.storage = unsupportedUsageStorage{}
	srvr.maxStorageSize = 1024

	err := srvr.initStorageQuota()
	if !errors.Is(err, storage.ErrUsageUnsupported) {
		t.Fatalf("initStorageQuota = %v, want ErrUsageUnsupported", err)
	}

	// Without a limit configured the same backend starts fine.
	srvr.maxStorageSize = 0
	if err := srvr.initStorageQuota(); err != nil {
		t.Errorf("unlimited instance refused to start: %v", err)
	}
}
