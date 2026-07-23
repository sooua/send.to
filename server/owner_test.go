package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

const testOwnerToken = "6VzKt0m9c1QnJ8xLpR4wYb2sTf7uHgAe3D"

func putWithOwner(t *testing.T, srvr *Server, filename, body, owner string, headers map[string]string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/"+filename, strings.NewReader(body))
	req.ContentLength = int64(len(body))
	if owner != "" {
		req.Header.Set("X-Owner-Token", owner)
	}
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

func ownerFiles(t *testing.T, srvr *Server, owner string) []ownerEntry {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/owner/files", nil)
	req.Header.Set("Accept", "application/json")
	if owner != "" {
		req.Header.Set("X-Owner-Token", owner)
	}

	w := httptest.NewRecorder()
	srvr.ownerFilesHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /owner/files = %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Files []ownerEntry `json:"files"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("could not decode the file list: %v", err)
	}

	return body.Files
}

func TestOwnerIndexListsAndForgets(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	first := putWithOwner(t, srvr, "a.txt", "alpha", testOwnerToken, nil)
	putWithOwner(t, srvr, "b.txt", "bravo", testOwnerToken, nil)

	// An upload without a token stays anonymous and must not appear.
	putWithOwner(t, srvr, "anon.txt", "nobody", "", nil)

	files := ownerFiles(t, srvr, testOwnerToken)
	if len(files) != 2 {
		t.Fatalf("owner has %d files, want 2: %+v", len(files), files)
	}
	if files[0].Filename != "b.txt" {
		t.Errorf("newest entry = %q, want b.txt", files[0].Filename)
	}
	if files[0].DeleteURL == "" {
		t.Error("the list carries no delete link, which is the point of it")
	}

	// A different secret is a different owner, with nothing in it.
	if other := ownerFiles(t, srvr, "Z"+testOwnerToken[1:]); len(other) != 0 {
		t.Errorf("a different token sees %d files", len(other))
	}

	// Deleting an upload takes it out of the list.
	parts := strings.Split(first, "/")
	uploadToken := parts[len(parts)-2]

	m, err := srvr.checkMetadata(t.Context(), uploadToken, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if m.OwnerHash == "" {
		t.Fatal("the upload recorded no owner hash")
	}
	if strings.Contains(m.OwnerHash, testOwnerToken) {
		t.Fatal("the owner token was stored rather than its hash")
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/"+uploadToken+"/a.txt/"+m.DeletionToken, nil)
	delReq = mux.SetURLVars(delReq, map[string]string{
		"token": uploadToken, "filename": "a.txt", "deletionToken": m.DeletionToken,
	})
	delRec := httptest.NewRecorder()
	srvr.deleteHandler(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE = %d: %s", delRec.Code, delRec.Body.String())
	}

	files = ownerFiles(t, srvr, testOwnerToken)
	if len(files) != 1 || files[0].Filename != "b.txt" {
		t.Errorf("after deleting a.txt the list is %+v", files)
	}
}

// An upload that runs out of downloads is deleted from storage, so it must not
// keep a link in the owner's list either.
func TestOwnerIndexDropsExhaustedUploads(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	uploadURL := putWithOwner(t, srvr, "once.txt", "single use", testOwnerToken, map[string]string{"Max-Downloads": "1"})

	parts := strings.Split(uploadURL, "/")
	uploadToken := parts[len(parts)-2]

	getReq := httptest.NewRequest(http.MethodGet, "/"+uploadToken+"/once.txt", nil)
	getReq = mux.SetURLVars(getReq, map[string]string{"token": uploadToken, "filename": "once.txt"})
	getRec := httptest.NewRecorder()
	srvr.getHandler(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET = %d", getRec.Code)
	}

	if files := ownerFiles(t, srvr, testOwnerToken); len(files) != 0 {
		t.Errorf("spent upload still listed: %+v", files)
	}
}

func TestOwnerTokenValidation(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Too short to be unguessable: refused rather than silently accepted.
	req := httptest.NewRequest(http.MethodGet, "/owner/files", nil)
	req.Header.Set("X-Owner-Token", "short")
	w := httptest.NewRecorder()
	srvr.ownerFilesHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("short token = %d, want 400", w.Code)
	}

	// And an upload carrying one is stored without an owner at all.
	url := putWithOwner(t, srvr, "weak.txt", "hello", "short", nil)

	parts := strings.Split(url, "/")
	m, err := srvr.checkMetadata(t.Context(), parts[len(parts)-2], "weak.txt")
	if err != nil {
		t.Fatal(err)
	}
	if m.OwnerHash != "" {
		t.Error("a too-short token was accepted as an owner")
	}
}

func TestOwnerFilesPlainTextListsURLs(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	uploadURL := putWithOwner(t, srvr, "c.txt", "charlie", testOwnerToken, nil)

	req := httptest.NewRequest(http.MethodGet, "/owner/files", nil)
	req.Header.Set("X-Owner-Token", testOwnerToken)
	w := httptest.NewRecorder()
	srvr.ownerFilesHandler(w, req)

	if got := strings.TrimSpace(w.Body.String()); got != uploadURL {
		t.Errorf("plain text list = %q, want %q", got, uploadURL)
	}
}
