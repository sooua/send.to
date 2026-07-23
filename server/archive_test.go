package server

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestParseArchiveKeys(t *testing.T) {
	tests := []struct {
		name      string
		files     string
		proxyPath string
		want      []archiveEntry
	}{
		{
			name:  "two entries",
			files: "tokenA/a.txt,tokenB/b.txt",
			want: []archiveEntry{
				{token: "tokenA", filename: "a.txt"},
				{token: "tokenB", filename: "b.txt"},
			},
		},
		{
			// Regression: this used to index [1] on a one-element slice and
			// panic, which the recovery middleware turned into a 500.
			name:  "missing separator is dropped",
			files: "abc",
			want:  nil,
		},
		{
			name:  "empty token or filename is dropped",
			files: "/a.txt,tokenB/",
			want:  nil,
		},
		{
			name:  "mixed valid and invalid",
			files: "abc,tokenB/b.txt",
			want:  []archiveEntry{{token: "tokenB", filename: "b.txt"}},
		},
		{
			name:  "filename is sanitized against traversal",
			files: "tokenA/../../etc/passwd",
			want:  []archiveEntry{{token: "tokenA", filename: "passwd"}},
		},
		{
			name:      "proxy path is stripped",
			files:     "/send/tokenA/a.txt",
			proxyPath: "send/",
			want:      []archiveEntry{{token: "tokenA", filename: "a.txt"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArchiveKeys(tt.files, tt.proxyPath)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries (%v), want %d (%v)", len(got), got, len(tt.want), tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// A malformed archive request must be a clean 400, not a recovered panic.
func TestZipHandlerMalformedRequest(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	req := httptest.NewRequest("GET", "/(abc).zip", nil)
	req = mux.SetURLVars(req, map[string]string{"files": "abc"})

	w := httptest.NewRecorder()
	srvr.zipHandler(w, req)

	if code := w.Result().StatusCode; code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
}

func TestZipHandlerBundlesUploads(t *testing.T) {
	srvr, tmpDir := newTestServer(t)
	defer os.RemoveAll(tmpDir)

	uploads := map[string]string{
		"one.txt": "first file",
		"two.txt": "second file",
	}

	var keys []string
	for filename, content := range uploads {
		req := httptest.NewRequest("PUT", "/"+filename, strings.NewReader(content))
		req.ContentLength = int64(len(content))
		req = mux.SetURLVars(req, map[string]string{"filename": filename})

		w := httptest.NewRecorder()
		srvr.putHandler(w, req)

		body, _ := io.ReadAll(w.Result().Body)
		parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(string(body)), "http://"), "/")
		keys = append(keys, parts[len(parts)-2]+"/"+filename)
	}

	files := strings.Join(keys, ",")
	req := httptest.NewRequest("GET", "/("+files+").zip", nil)
	req = mux.SetURLVars(req, map[string]string{"files": files})

	w := httptest.NewRecorder()
	srvr.zipHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	raw, _ := io.ReadAll(resp.Body)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("response is not a valid zip: %v", err)
	}

	if len(zr.File) != len(uploads) {
		t.Fatalf("zip contains %d entries, want %d", len(zr.File), len(uploads))
	}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("could not open %s: %v", f.Name, err)
		}
		content, _ := io.ReadAll(rc)
		_ = rc.Close()

		if want := uploads[f.Name]; string(content) != want {
			t.Errorf("%s = %q, want %q", f.Name, content, want)
		}
	}
}
