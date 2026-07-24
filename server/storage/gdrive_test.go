package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// The Google Drive backend is exercised against a fake Drive API rather than a
// real account: what has broken here is query construction, not the network.
// Purge shipped for years scoped to the wrong parent directory, matching
// nothing and reporting success, and no test could have noticed without
// inspecting the queries this backend actually sends.

type fakeDriveFile struct {
	ID           string
	Name         string
	MimeType     string
	Parents      []string
	ModifiedTime time.Time
	Trashed      bool
}

// fakeDrive serves the handful of Drive v3 endpoints this backend uses, over an
// in-memory file table.
type fakeDrive struct {
	mu     sync.Mutex
	files  map[string]*fakeDriveFile
	nextID int

	// listCalls records every q= this backend sent, so a test can assert on the
	// query and not only on its effect.
	listCalls []string
}

func newFakeDrive() *fakeDrive {
	return &fakeDrive{files: map[string]*fakeDriveFile{}}
}

func (d *fakeDrive) add(f *fakeDriveFile) *fakeDriveFile {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.nextID++
	f.ID = fmt.Sprintf("id%d", d.nextID)
	if f.ModifiedTime.IsZero() {
		f.ModifiedTime = time.Now()
	}
	d.files[f.ID] = f

	return f
}

func (d *fakeDrive) exists(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, ok := d.files[id]
	return ok
}

func (d *fakeDrive) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	return len(d.files)
}

// matches evaluates the subset of the Drive query language this backend emits.
// Anything it does not understand fails the test rather than matching
// silently — a query that quietly means something else is the bug being
// guarded against.
func (d *fakeDrive) matches(t *testing.T, f *fakeDriveFile, query string) bool {
	t.Helper()

	for _, clause := range strings.Split(query, " and ") {
		clause = strings.TrimSpace(clause)

		switch {
		case strings.HasSuffix(clause, " in parents"):
			want := strings.Trim(strings.TrimSuffix(clause, " in parents"), "'")
			if !contains(f.Parents, want) {
				return false
			}
		case clause == "trashed=false":
			if f.Trashed {
				return false
			}
		case strings.HasPrefix(clause, "name="):
			if f.Name != strings.Trim(strings.TrimPrefix(clause, "name="), "'") {
				return false
			}
		case strings.HasPrefix(clause, "mimeType!="):
			if f.MimeType == strings.Trim(strings.TrimPrefix(clause, "mimeType!="), "'") {
				return false
			}
		case strings.HasPrefix(clause, "mimeType="):
			if f.MimeType != strings.Trim(strings.TrimPrefix(clause, "mimeType="), "'") {
				return false
			}
		case strings.HasPrefix(clause, "modifiedTime < "):
			cutoff, err := time.Parse(time.RFC3339, strings.Trim(strings.TrimPrefix(clause, "modifiedTime < "), "'"))
			if err != nil {
				t.Fatalf("unparsable modifiedTime in query %q: %v", query, err)
			}
			if !f.ModifiedTime.Before(cutoff) {
				return false
			}
		default:
			t.Fatalf("fake Drive does not understand query clause %q (full query %q)", clause, query)
		}
	}

	return true
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func (d *fakeDrive) handler(t *testing.T) http.Handler {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			query := r.URL.Query().Get("q")

			d.mu.Lock()
			d.listCalls = append(d.listCalls, query)
			candidates := make([]*fakeDriveFile, 0, len(d.files))
			for _, f := range d.files {
				candidates = append(candidates, f)
			}
			d.mu.Unlock()

			result := &drive.FileList{}
			for _, f := range candidates {
				if d.matches(t, f, query) {
					result.Files = append(result.Files, &drive.File{Id: f.ID, Name: f.Name, MimeType: f.MimeType})
				}
			}

			writeJSON(t, w, result)

		case http.MethodPost:
			var body drive.File
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("create: %v", err)
			}

			created := d.add(&fakeDriveFile{Name: body.Name, MimeType: body.MimeType, Parents: body.Parents})
			writeJSON(t, w, &drive.File{Id: created.ID, Name: created.Name})

		default:
			t.Fatalf("unexpected %s /files", r.Method)
		}
	})

	// Media uploads go to a separate absolute path, and arrive as
	// multipart/related with the file metadata as the first part.
	mux.HandleFunc("/upload/drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Fatalf("unexpected upload content type %q (%v)", r.Header.Get("Content-Type"), err)
		}

		part, err := multipart.NewReader(r.Body, params["boundary"]).NextPart()
		if err != nil {
			t.Fatalf("upload metadata part: %v", err)
		}

		var body drive.File
		if err := json.NewDecoder(part).Decode(&body); err != nil {
			t.Fatalf("upload metadata: %v", err)
		}

		created := d.add(&fakeDriveFile{Name: body.Name, MimeType: body.MimeType, Parents: body.Parents})
		writeJSON(t, w, &drive.File{Id: created.ID, Name: created.Name})
	})

	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/files/")

		switch r.Method {
		case http.MethodDelete:
			d.mu.Lock()
			_, ok := d.files[id]
			delete(d.files, id)
			d.mu.Unlock()

			if !ok {
				http.Error(w, `{"error":{"code":404,"message":"not found"}}`, http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case http.MethodGet:
			d.mu.Lock()
			f, ok := d.files[id]
			d.mu.Unlock()

			if !ok {
				http.Error(w, `{"error":{"code":404,"message":"not found"}}`, http.StatusNotFound)
				return
			}
			writeJSON(t, w, &drive.File{Id: f.ID, Name: f.Name, Size: 1234, Md5Checksum: "deadbeef"})

		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	return mux
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func gdriveTestStorage(t *testing.T) (*GDrive, *fakeDrive) {
	t.Helper()

	fake := newFakeDrive()
	srv := httptest.NewServer(fake.handler(t))
	t.Cleanup(srv.Close)

	service, err := drive.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/"),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}

	root := fake.add(&fakeDriveFile{Name: "sendto", MimeType: gDriveDirectoryMimeType})

	return &GDrive{
		service:   service,
		rootID:    root.ID,
		basedir:   "sendto",
		chunkSize: 1 << 20,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, fake
}

func TestGDrivePurgeRemovesExpiredFilesInTokenDirectories(t *testing.T) {
	store, fake := gdriveTestStorage(t)

	old := time.Now().Add(-72 * time.Hour)

	stale := fake.add(&fakeDriveFile{Name: "stale.bin", MimeType: "application/octet-stream", ModifiedTime: old})
	staleMeta := fake.add(&fakeDriveFile{Name: "stale.bin.metadata", MimeType: "text/json", ModifiedTime: old})
	staleDir := fake.add(&fakeDriveFile{Name: "tokenold", MimeType: gDriveDirectoryMimeType, Parents: []string{store.rootID}, ModifiedTime: old})
	stale.Parents = []string{staleDir.ID}
	staleMeta.Parents = []string{staleDir.ID}

	fresh := fake.add(&fakeDriveFile{Name: "fresh.bin", MimeType: "application/octet-stream"})
	freshDir := fake.add(&fakeDriveFile{Name: "tokennew", MimeType: gDriveDirectoryMimeType, Parents: []string{store.rootID}})
	fresh.Parents = []string{freshDir.ID}

	if err := store.Purge(t.Context(), 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	if fake.exists(stale.ID) {
		t.Error("expired upload survived the purge — this is what --purge-days is for")
	}
	if fake.exists(staleMeta.ID) {
		t.Error("expired upload's metadata sidecar survived the purge")
	}
	if fake.exists(staleDir.ID) {
		t.Error("emptied token directory survived the purge; one is left behind per expired upload")
	}
	if !fake.exists(fresh.ID) {
		t.Error("purge deleted a file that had not expired")
	}
	if !fake.exists(freshDir.ID) {
		t.Error("purge deleted a token directory that still holds a file")
	}
	if !fake.exists(store.rootID) {
		t.Error("purge deleted its own root directory")
	}
}

func TestGDrivePurgeScopesToTokenDirectoriesNotRoot(t *testing.T) {
	store, fake := gdriveTestStorage(t)

	if err := store.Purge(t.Context(), 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	// The original implementation asked only about files parented directly by
	// the root, where uploads never live. If the sole query is that one, purge
	// is back to being a no-op that reports success.
	for _, q := range fake.listCalls {
		if strings.Contains(q, "modifiedTime <") && strings.Contains(q, "'"+store.rootID+"' in parents") {
			t.Errorf("purge looked for expired files directly under the root, where Put never stores any: %q", q)
		}
	}
}

func TestGDrivePurgeLeavesUnexpiredAccountsAlone(t *testing.T) {
	store, fake := gdriveTestStorage(t)

	// A directory that is not the backend's root: purge must never widen its
	// query to the whole Drive account.
	unrelated := fake.add(&fakeDriveFile{Name: "holiday-photos", MimeType: gDriveDirectoryMimeType})
	photo := fake.add(&fakeDriveFile{
		Name:         "beach.jpg",
		MimeType:     "image/jpeg",
		Parents:      []string{unrelated.ID},
		ModifiedTime: time.Now().Add(-10000 * time.Hour),
	})

	if err := store.Purge(t.Context(), time.Hour); err != nil {
		t.Fatal(err)
	}

	if !fake.exists(photo.ID) {
		t.Fatal("purge deleted a file outside the storage root")
	}
}

func TestGDriveDeleteRemovesFileAndMetadata(t *testing.T) {
	store, fake := gdriveTestStorage(t)

	dir := fake.add(&fakeDriveFile{Name: "tok", MimeType: gDriveDirectoryMimeType, Parents: []string{store.rootID}})
	file := fake.add(&fakeDriveFile{Name: "a.bin", MimeType: "application/octet-stream", Parents: []string{dir.ID}})
	meta := fake.add(&fakeDriveFile{Name: "a.bin.metadata", MimeType: "text/json", Parents: []string{dir.ID}})

	if err := store.Delete(t.Context(), "tok", "a.bin"); err != nil {
		t.Fatal(err)
	}

	if fake.exists(file.ID) {
		t.Error("Delete left the file behind")
	}
	if fake.exists(meta.ID) {
		t.Error("Delete left the metadata sidecar behind, so the token still looks alive")
	}
}

func TestGDriveHeadReadsSizeThroughTokenDirectory(t *testing.T) {
	store, fake := gdriveTestStorage(t)

	dir := fake.add(&fakeDriveFile{Name: "tok", MimeType: gDriveDirectoryMimeType, Parents: []string{store.rootID}})
	fake.add(&fakeDriveFile{Name: "a.bin", MimeType: "application/octet-stream", Parents: []string{dir.ID}})

	length, err := store.Head(t.Context(), "tok", "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if length != 1234 {
		t.Errorf("Head = %d, want 1234", length)
	}
}

func TestGDriveHeadOnMissingFile(t *testing.T) {
	store, _ := gdriveTestStorage(t)

	if _, err := store.Head(t.Context(), "nosuch", "a.bin"); err == nil {
		t.Fatal("Head on a missing file returned no error")
	}
}

func TestGDriveUsageIsUnsupported(t *testing.T) {
	store, _ := gdriveTestStorage(t)

	if _, err := store.Usage(t.Context()); err != ErrUsageUnsupported {
		t.Fatalf("Usage error = %v, want ErrUsageUnsupported — the storage quota must refuse to start, not silently allow everything", err)
	}
}

func TestGDrivePutCreatesTokenDirectoryOnce(t *testing.T) {
	store, fake := gdriveTestStorage(t)

	before := fake.count()

	if err := store.Put(t.Context(), "tok", "a.bin", strings.NewReader("hello"), "text/plain", 5); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), "tok", "b.bin", strings.NewReader("hello"), "text/plain", 5); err != nil {
		t.Fatal(err)
	}

	// One directory plus two files.
	if got := fake.count() - before; got != 3 {
		t.Errorf("two uploads to one token created %d objects, want 3 (one shared directory)", got)
	}
}
