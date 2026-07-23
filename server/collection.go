package server

// Collections: one link for several files.
//
// Uploading five log files produces five links, and pasting five links into a
// chat is how the fifth one gets lost. The archive syntax the server already
// had — `/(tokenA/a.log,tokenB/b.log).zip` — solves the download side but has to
// be assembled by hand from tokens nobody wrote down.
//
// A collection is a small object naming uploads that already exist:
//
//	POST /collection    {"name": "nightly", "files": ["<url>", …]}
//	GET  /c/{token}     landing page, JSON, or one URL per line for curl
//	GET  /c/{token}.zip everything in one archive
//
// It owns no bytes of its own. Deleting it leaves the files alone, deleting a
// file leaves the collection valid for the rest, and a collection whose files
// have all expired reports 404 and takes itself out of storage.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sooua/send.to/server/storage"
)

const (
	// collectionObject is the filename a collection is stored under, inside
	// its own token directory.
	collectionObject = "collection.json"

	// collectionMaxFiles bounds one collection. The archive route streams
	// every member in a single request, so this is also a cap on how much one
	// download can ask the storage backend for.
	collectionMaxFiles = 100

	// collectionMaxBody is plenty for a hundred URLs and small enough that an
	// unauthenticated POST cannot be used to buffer megabytes.
	collectionMaxBody = 64 << 10
)

// collectionFile is one member, stored by token and name rather than by URL so
// a collection keeps working when the instance moves to another hostname.
type collectionFile struct {
	Token       string `json:"token"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Encrypted   bool   `json:"encrypted"`
}

// collectionData is the stored object. It is never served as-is: the deletion
// token lives here, and the read handler builds its own response.
type collectionData struct {
	Name          string           `json:"name,omitempty"`
	Files         []collectionFile `json:"files"`
	DeletionToken string           `json:"deletion_token"`
	CreatedAt     time.Time        `json:"created_at"`
}

// collectionFileView is one member as a client sees it.
type collectionFileView struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Encrypted   bool   `json:"encrypted"`
}

// collectionView is the response for both creating and reading a collection.
type collectionView struct {
	URL        string               `json:"url"`
	DeleteURL  string               `json:"delete_url,omitempty"`
	ArchiveURL string               `json:"archive_url"`
	Name       string               `json:"name,omitempty"`
	Files      []collectionFileView `json:"files"`
	TotalSize  int64                `json:"total_size"`
	CreatedAt  time.Time            `json:"created_at"`
}

// parseCollectionRef turns whatever a client sent — a full share URL, a
// `token/filename` pair, a `/get/token/filename` link — into the two parts the
// storage layer needs.
func parseCollectionRef(ref, proxyPath string) (token, filename string, ok bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false
	}

	// A fragment carries an end-to-end encryption key; it never belongs in a
	// request, and a collection cannot use it, so drop it here too.
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref = ref[:i]
	}

	if u, err := url.Parse(ref); err == nil && u.Path != "" {
		ref = u.Path
	}

	ref = resolveKey(ref, proxyPath)

	parts := strings.Split(strings.Trim(ref, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}

	token = parts[len(parts)-2]
	filename = sanitize(parts[len(parts)-1])

	if token == "" || filename == "" || strings.Contains(token, ".") {
		return "", "", false
	}

	if unescaped, err := url.PathUnescape(filename); err == nil {
		filename = sanitize(unescaped)
	}

	return token, filename, true
}

func (s *Server) readCollection(ctx context.Context, token string) (*collectionData, error) {
	r, _, err := s.storage.Get(ctx, token, collectionObject, nil)
	defer storage.CloseCheck(r)

	if err != nil {
		return nil, err
	}

	var data collectionData
	if err := json.NewDecoder(r).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

func (s *Server) writeCollection(ctx context.Context, token string, data *collectionData) error {
	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(data); err != nil {
		return err
	}

	return s.storage.Put(ctx, token, collectionObject, buffer, "application/json", uint64(buffer.Len()))
}

// collectionViewFor builds the response, resolving every URL against the host
// the request arrived on.
func (s *Server) collectionViewFor(r *http.Request, token string, data *collectionData, includeDelete bool) collectionView {
	view := collectionView{
		Name:      data.Name,
		CreatedAt: data.CreatedAt,
		Files:     make([]collectionFileView, 0, len(data.Files)),
	}

	collectionURL, _ := url.Parse(path.Join(s.proxyPath, "c", token))
	view.URL = resolveURL(r, collectionURL, s.proxyPort)
	view.ArchiveURL = view.URL + ".zip"

	if includeDelete {
		deleteURL, _ := url.Parse(path.Join(s.proxyPath, "c", token, data.DeletionToken))
		view.DeleteURL = resolveURL(r, deleteURL, s.proxyPort)
	}

	for _, f := range data.Files {
		fileURL, _ := url.Parse(path.Join(s.proxyPath, f.Token, url.PathEscape(f.Filename)))

		view.Files = append(view.Files, collectionFileView{
			URL:         resolveURL(r, fileURL, s.proxyPort),
			Filename:    f.Filename,
			Size:        f.Size,
			ContentType: f.ContentType,
			Encrypted:   f.Encrypted,
		})

		view.TotalSize += f.Size
	}

	return view
}

// createCollectionHandler groups uploads that already exist into one link.
func (s *Server) createCollectionHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
	}

	if err := json.NewDecoder(io.LimitReader(r.Body, collectionMaxBody)).Decode(&body); err != nil {
		http.Error(w, "Body must be JSON: {\"files\": [\"<url>\", …]}", http.StatusBadRequest)
		return
	}

	if len(body.Files) == 0 {
		http.Error(w, "A collection needs at least one file", http.StatusBadRequest)
		return
	}

	if len(body.Files) > collectionMaxFiles {
		http.Error(w, fmt.Sprintf("A collection holds at most %d files", collectionMaxFiles), http.StatusBadRequest)
		return
	}

	data := &collectionData{
		Name:          sanitizeCollectionName(body.Name),
		DeletionToken: token(s.randomTokenLength) + token(s.randomTokenLength),
		CreatedAt:     time.Now().UTC(),
	}

	seen := make(map[string]bool, len(body.Files))

	for _, ref := range body.Files {
		fileToken, filename, ok := parseCollectionRef(ref, s.proxyPath)
		if !ok {
			http.Error(w, fmt.Sprintf("%q is not a share link from this server", ref), http.StatusBadRequest)
			return
		}

		key := fileToken + "/" + filename
		if seen[key] {
			continue
		}
		seen[key] = true

		// Checking the metadata proves the upload exists and is still live,
		// and does not spend a download from its budget.
		m, err := s.checkMetadata(r.Context(), fileToken, filename)
		if err != nil {
			http.Error(w, fmt.Sprintf("%s is not available", filename), http.StatusNotFound)
			return
		}

		data.Files = append(data.Files, collectionFile{
			Token:       fileToken,
			Filename:    filename,
			Size:        m.ContentLength,
			ContentType: m.ContentType,
			Encrypted:   m.Encrypted,
		})
	}

	collectionToken := token(s.randomTokenLength)

	if err := s.writeCollection(r.Context(), collectionToken, data); err != nil {
		s.logger.Error("Could not store collection", "error", err)
		http.Error(w, "Could not create the collection", http.StatusInternalServerError)
		return
	}

	view := s.collectionViewFor(r, collectionToken, data, true)

	s.logger.Info("Collection created", "token", maskToken(collectionToken), "files", len(data.Files))

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Url-Delete", view.DeleteURL)

	if wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(view)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(view.URL))
}

// sanitizeCollectionName keeps a title that is safe to render and short enough
// to be a title.
func sanitizeCollectionName(name string) string {
	name = sanitize(strings.TrimSpace(name))
	if name == "_" {
		return ""
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

// liveCollection loads a collection and drops members whose upload has gone.
// A collection with nothing left in it is deleted and reported as missing,
// so a stale link never renders an empty page.
func (s *Server) liveCollection(w http.ResponseWriter, r *http.Request, collectionToken string) (*collectionData, bool) {
	data, err := s.readCollection(r.Context(), collectionToken)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil, false
	}

	live := make([]collectionFile, 0, len(data.Files))
	for _, f := range data.Files {
		if _, err := s.checkMetadata(r.Context(), f.Token, f.Filename); err != nil {
			continue
		}
		live = append(live, f)
	}

	if len(live) != len(data.Files) {
		data.Files = live

		if len(live) == 0 {
			if err := s.storage.Delete(r.Context(), collectionToken, collectionObject); err != nil && !s.storage.IsNotExist(err) {
				s.logger.Error("Could not delete an empty collection", "token", maskToken(collectionToken), "error", err)
			}
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return nil, false
		}

		if err := s.writeCollection(r.Context(), collectionToken, data); err != nil {
			s.logger.Error("Could not prune collection", "token", maskToken(collectionToken), "error", err)
		}
	}

	return data, true
}

// collectionHandler answers a collection link: the landing page for a browser,
// JSON for a program, one URL per line for a shell loop.
func (s *Server) collectionHandler(w http.ResponseWriter, r *http.Request) {
	collectionToken := mux.Vars(r)["token"]

	data, ok := s.liveCollection(w, r, collectionToken)
	if !ok {
		return
	}

	view := s.collectionViewFor(r, collectionToken, data, false)

	w.Header().Set("Vary", "Accept")
	w.Header().Set("Cache-Control", "no-store")

	if wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
		return
	}

	// The Astro frontend renders the list from the JSON above; serve it when
	// this build has one.
	if acceptsHTML(r.Header) {
		if page := s.collectionPagePath(); page != "" {
			http.ServeFile(w, r, page)
			return
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, f := range view.Files {
		_, _ = fmt.Fprintln(w, f.URL)
	}
}

// collectionPagePath returns the Astro collection page, or "" when the frontend
// does not provide one. Resolved per request so --web-path can be swapped
// without a restart, matching the preview page.
func (s *Server) collectionPagePath() string {
	if s.webPath == "" {
		return ""
	}

	page := filepath.Join(s.webPath, "collection", "index.html")
	if _, err := os.Stat(page); err != nil {
		return ""
	}

	return page
}

// collectionArchiveHandler redirects to the archive route, which already knows
// how to stream several stored objects as one file. Members are charged a
// download each, exactly as if they had been fetched individually.
func (s *Server) collectionArchiveHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	data, ok := s.liveCollection(w, r, vars["token"])
	if !ok {
		return
	}

	keys := make([]string, 0, len(data.Files))
	for _, f := range data.Files {
		keys = append(keys, path.Join(f.Token, f.Filename))
	}

	archive, _ := url.Parse(path.Join(s.proxyPath, fmt.Sprintf("(%s).%s", strings.Join(keys, ","), vars["format"])))

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, resolveURL(r, archive, s.proxyPort), http.StatusFound)
}

// deleteCollectionHandler removes the collection object. The files it named are
// left alone: they have their own deletion links, and somebody else may hold
// one of those links directly.
func (s *Server) deleteCollectionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	collectionToken := vars["token"]

	data, err := s.readCollection(r.Context(), collectionToken)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	if data.DeletionToken != vars["deletionToken"] {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	if err := s.storage.Delete(r.Context(), collectionToken, collectionObject); err != nil && !s.storage.IsNotExist(err) {
		s.logger.Error("Could not delete collection", "token", maskToken(collectionToken), "error", err)
		http.Error(w, "Could not delete the collection", http.StatusInternalServerError)
		return
	}

	s.metrics.deletes.Add(1)
}
