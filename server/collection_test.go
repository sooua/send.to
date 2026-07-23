package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// uploadForCollection stores a file and returns its share URL.
func uploadForCollection(t *testing.T, srvr *Server, filename, body string, headers map[string]string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/"+filename, strings.NewReader(body))
	req.ContentLength = int64(len(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req = mux.SetURLVars(req, map[string]string{"filename": filename})

	w := httptest.NewRecorder()
	srvr.putHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT %s = %d: %s", filename, w.Code, w.Body.String())
	}

	return strings.TrimSpace(w.Body.String())
}

func createCollection(t *testing.T, srvr *Server, name string, files []string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
	}{name, files})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader(string(body)))
	req.Header.Set("Accept", "application/json")

	w := httptest.NewRecorder()
	srvr.createCollectionHandler(w, req)

	return w
}

func readCollectionView(t *testing.T, srvr *Server, collectionToken, accept string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/c/"+collectionToken, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req = mux.SetURLVars(req, map[string]string{"token": collectionToken})

	w := httptest.NewRecorder()
	srvr.collectionHandler(w, req)

	return w
}

func tokenFromURL(url string) string {
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func TestCollectionRoundTrip(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	first := uploadForCollection(t, srvr, "a.log", "alpha", nil)
	second := uploadForCollection(t, srvr, "b.log", "bravo bravo", nil)

	created := createCollection(t, srvr, "nightly", []string{first, second})
	if created.Code != http.StatusCreated {
		t.Fatalf("POST /collection = %d: %s", created.Code, created.Body.String())
	}

	var view collectionView
	if err := json.NewDecoder(created.Body).Decode(&view); err != nil {
		t.Fatalf("could not decode the collection: %v", err)
	}

	if len(view.Files) != 2 {
		t.Fatalf("collection has %d files, want 2", len(view.Files))
	}
	if view.Name != "nightly" {
		t.Errorf("name = %q, want nightly", view.Name)
	}
	if view.TotalSize != int64(len("alpha")+len("bravo bravo")) {
		t.Errorf("total size = %d", view.TotalSize)
	}
	if view.ArchiveURL != view.URL+".zip" {
		t.Errorf("archive URL = %q", view.ArchiveURL)
	}
	if created.Header().Get("X-Url-Delete") == "" {
		t.Error("no delete link returned")
	}

	collectionToken := tokenFromURL(view.URL)

	// JSON view: the same list, without the deletion token in it.
	read := readCollectionView(t, srvr, collectionToken, "application/json")
	if read.Code != http.StatusOK {
		t.Fatalf("GET /c/%s = %d", collectionToken, read.Code)
	}
	if strings.Contains(read.Body.String(), "deletion_token") {
		t.Error("the collection view leaks its deletion token")
	}

	var fetched collectionView
	if err := json.NewDecoder(strings.NewReader(read.Body.String())).Decode(&fetched); err != nil {
		t.Fatal(err)
	}
	if len(fetched.Files) != 2 || fetched.Files[0].Filename != "a.log" {
		t.Errorf("fetched files = %+v", fetched.Files)
	}
	if fetched.DeleteURL != "" {
		t.Error("the read view offers a delete link it should not know about")
	}

	// Plain text: one URL per line, which is what a shell loop consumes.
	text := readCollectionView(t, srvr, collectionToken, "")
	lines := strings.Fields(strings.TrimSpace(text.Body.String()))
	if len(lines) != 2 || lines[0] != first || lines[1] != second {
		t.Errorf("text view = %q", text.Body.String())
	}

	// The archive link redirects onto the existing multi-file route.
	archiveReq := httptest.NewRequest(http.MethodGet, "/c/"+collectionToken+".zip", nil)
	archiveReq = mux.SetURLVars(archiveReq, map[string]string{"token": collectionToken, "format": "zip"})
	archiveRec := httptest.NewRecorder()
	srvr.collectionArchiveHandler(archiveRec, archiveReq)

	if archiveRec.Code != http.StatusFound {
		t.Fatalf("archive = %d, want 302", archiveRec.Code)
	}
	location := archiveRec.Header().Get("Location")
	if !strings.Contains(location, "a.log,") || !strings.HasSuffix(location, ").zip") {
		t.Errorf("archive redirect = %q", location)
	}
}

// Deleting a collection must not touch the files it named: somebody may hold
// one of those links directly.
func TestCollectionDeleteLeavesFiles(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	fileURL := uploadForCollection(t, srvr, "keep.log", "still here", nil)

	created := createCollection(t, srvr, "", []string{fileURL})
	var view collectionView
	_ = json.NewDecoder(created.Body).Decode(&view)

	collectionToken := tokenFromURL(view.URL)
	deletionToken := created.Header().Get("X-Url-Delete")
	deletionToken = deletionToken[strings.LastIndex(deletionToken, "/")+1:]

	t.Run("wrong deletion token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/c/"+collectionToken+"/nope", nil)
		req = mux.SetURLVars(req, map[string]string{"token": collectionToken, "deletionToken": "nope"})
		w := httptest.NewRecorder()
		srvr.deleteCollectionHandler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	req := httptest.NewRequest(http.MethodDelete, "/c/"+collectionToken+"/"+deletionToken, nil)
	req = mux.SetURLVars(req, map[string]string{"token": collectionToken, "deletionToken": deletionToken})
	w := httptest.NewRecorder()
	srvr.deleteCollectionHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DELETE = %d: %s", w.Code, w.Body.String())
	}

	if got := readCollectionView(t, srvr, collectionToken, "application/json").Code; got != http.StatusNotFound {
		t.Errorf("deleted collection = %d, want 404", got)
	}

	// The file is still there.
	parts := strings.Split(fileURL, "/")
	if _, err := srvr.checkMetadata(t.Context(), parts[len(parts)-2], "keep.log"); err != nil {
		t.Errorf("deleting the collection took the file with it: %v", err)
	}
}

// A collection heals itself: members that expire drop out, and a collection
// with nothing left reports 404 rather than rendering an empty page.
func TestCollectionDropsDeadMembers(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	spent := uploadForCollection(t, srvr, "once.log", "single use", map[string]string{"Max-Downloads": "1"})
	kept := uploadForCollection(t, srvr, "kept.log", "still here", nil)

	created := createCollection(t, srvr, "", []string{spent, kept})
	var view collectionView
	_ = json.NewDecoder(created.Body).Decode(&view)
	collectionToken := tokenFromURL(view.URL)

	// Spend the one download the first file had.
	parts := strings.Split(spent, "/")
	spentToken := parts[len(parts)-2]
	getReq := httptest.NewRequest(http.MethodGet, spent, nil)
	getReq = mux.SetURLVars(getReq, map[string]string{"token": spentToken, "filename": "once.log"})
	srvr.getHandler(httptest.NewRecorder(), getReq)

	read := readCollectionView(t, srvr, collectionToken, "application/json")
	var fetched collectionView
	_ = json.NewDecoder(read.Body).Decode(&fetched)

	if len(fetched.Files) != 1 || fetched.Files[0].Filename != "kept.log" {
		t.Fatalf("collection still lists %+v", fetched.Files)
	}

	// Now delete the survivor: the collection has nothing to show.
	m, err := srvr.checkMetadata(t.Context(), strings.Split(kept, "/")[len(strings.Split(kept, "/"))-2], "kept.log")
	if err != nil {
		t.Fatal(err)
	}
	keptToken := strings.Split(kept, "/")[len(strings.Split(kept, "/"))-2]

	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/%s/kept.log/%s", keptToken, m.DeletionToken), nil)
	delReq = mux.SetURLVars(delReq, map[string]string{
		"token": keptToken, "filename": "kept.log", "deletionToken": m.DeletionToken,
	})
	srvr.deleteHandler(httptest.NewRecorder(), delReq)

	if got := readCollectionView(t, srvr, collectionToken, "application/json").Code; got != http.StatusNotFound {
		t.Errorf("empty collection = %d, want 404", got)
	}
}

func TestCollectionRejections(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	t.Run("empty", func(t *testing.T) {
		if code := createCollection(t, srvr, "", nil).Code; code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
	})

	t.Run("unknown file", func(t *testing.T) {
		if code := createCollection(t, srvr, "", []string{"http://example.com/aaaaaaaaaa/gone.log"}).Code; code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
	})

	t.Run("not a share link", func(t *testing.T) {
		if code := createCollection(t, srvr, "", []string{"nonsense"}).Code; code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
	})

	t.Run("too many", func(t *testing.T) {
		refs := make([]string, collectionMaxFiles+1)
		for i := range refs {
			refs[i] = "aaaaaaaaaa/x.log"
		}
		if code := createCollection(t, srvr, "", refs).Code; code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", code)
		}
	})

	t.Run("missing collection", func(t *testing.T) {
		if code := readCollectionView(t, srvr, "aaaaaaaaaa", "application/json").Code; code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
	})
}

func TestParseCollectionRef(t *testing.T) {
	cases := []struct {
		in       string
		token    string
		filename string
		ok       bool
	}{
		{"https://send.to/aB3cD4eF/notes.md", "aB3cD4eF", "notes.md", true},
		{"aB3cD4eF/notes.md", "aB3cD4eF", "notes.md", true},
		{"/get/aB3cD4eF/notes.md", "aB3cD4eF", "notes.md", true},
		{"https://send.to/aB3cD4eF/notes.md#k=abc", "aB3cD4eF", "notes.md", true},
		{"https://send.to/aB3cD4eF/my%20notes.md", "aB3cD4eF", "my notes.md", true},
		{"notes.md", "", "", false},
		{"", "", "", false},
		{"/", "", "", false},
	}

	for _, tc := range cases {
		token, filename, ok := parseCollectionRef(tc.in, "")
		if ok != tc.ok || token != tc.token || filename != tc.filename {
			t.Errorf("%q = (%q, %q, %v), want (%q, %q, %v)", tc.in, token, filename, ok, tc.token, tc.filename, tc.ok)
		}
	}
}
