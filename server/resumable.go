package server

// Resumable uploads.
//
// A PUT that dies at 90% of a 5 GB file has to start over from zero, which is
// the worst property `curl --upload-file` has over a flaky link — and pulling a
// build artefact off a production box is exactly where the link is flaky.
//
// The protocol is three calls on top of the same storage:
//
//	POST   /upload/{filename}       Upload-Length: <total>    → session URL
//	PATCH  /upload/{id}/{filename}  Content-Range: bytes a-b/total
//	HEAD   /upload/{id}/{filename}                            → Upload-Offset
//	DELETE /upload/{id}/{filename}                            → abandon it
//
// Bytes accumulate in a spool file under TEMP_PATH and reach the storage
// backend in one piece when the final chunk lands, so no backend needs append
// support and an interrupted upload never becomes a visible half file.
//
// A PATCH that fails part way through is rolled back to the offset it started
// at, so the offset a client resumes from is always a chunk boundary that the
// client itself chose. That is what keeps client-side encryption resumable: the
// ciphertext has to be regenerated from a known point, and an arbitrary byte
// offset inside an AEAD chunk is not one.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const (
	// sessionIDLength is independent of --random-token-length: a session URL is
	// a write capability on someone else's upload, so it is always long.
	sessionIDLength = 24

	// sessionTTL is how long an untouched session survives. Long enough to
	// carry a transfer across a laptop lid, short enough that abandoned spool
	// files do not accumulate on TEMP_PATH forever.
	sessionTTL = 24 * time.Hour

	// sessionSweepInterval bounds how often the amortised sweep runs.
	sessionSweepInterval = time.Hour

	// sessionLockToken namespaces session locks in the per-upload lock map.
	sessionLockToken = ".sessions"
)

// uploadSession is the sidecar describing one in-progress upload. It holds no
// secrets: a server-side encryption password would have to be stored in clear
// to survive between chunks, so resumable uploads reject it outright.
type uploadSession struct {
	ID           string    `json:"id"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"content_type"`
	Total        int64     `json:"total"`
	MaxDays      int       `json:"max_days,omitempty"`
	MaxDownloads int       `json:"max_downloads,omitempty"`
	OwnerHash    string    `json:"owner_hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// sessionIDPattern is what a session id may look like. It is checked before the
// id is ever joined onto a path.
var sessionIDPattern = regexp.MustCompile(`^[0-9a-zA-Z]{8,64}$`)

// contentRangePattern matches the only Content-Range form an upload may use:
// a fully specified byte span with a known total.
var contentRangePattern = regexp.MustCompile(`^bytes (\d+)-(\d+)/(\d+)$`)

func parseUploadContentRange(v string) (start, end, total int64, err error) {
	m := contentRangePattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, 0, 0, errors.New("Content-Range must look like `bytes <start>-<end>/<total>`")
	}

	if start, err = strconv.ParseInt(m[1], 10, 64); err != nil {
		return 0, 0, 0, errors.New("Content-Range start is out of range")
	}
	if end, err = strconv.ParseInt(m[2], 10, 64); err != nil {
		return 0, 0, 0, errors.New("Content-Range end is out of range")
	}
	if total, err = strconv.ParseInt(m[3], 10, 64); err != nil {
		return 0, 0, 0, errors.New("Content-Range total is out of range")
	}

	if end < start || end >= total {
		return 0, 0, 0, errors.New("Content-Range is not a valid span of the declared total")
	}

	return start, end, total, nil
}

// sessionDir is where spool files and sidecars live. An empty --temp-path means
// the system temp directory, matching what os.CreateTemp does elsewhere.
func (s *Server) sessionDir() string {
	base := s.tempPath
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "sendto-sessions")
}

func (s *Server) sessionPaths(id string) (part, sidecar string) {
	dir := s.sessionDir()
	return filepath.Join(dir, id+".part"), filepath.Join(dir, id+".json")
}

func (s *Server) loadSession(id string) (*uploadSession, error) {
	_, sidecar := s.sessionPaths(id)

	data, err := os.ReadFile(sidecar) //nolint:gosec // id is validated against sessionIDPattern
	if err != nil {
		return nil, err
	}

	var sess uploadSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}

	return &sess, nil
}

func (s *Server) saveSession(sess *uploadSession) error {
	if err := os.MkdirAll(s.sessionDir(), 0700); err != nil {
		return err
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}

	_, sidecar := s.sessionPaths(sess.ID)
	return os.WriteFile(sidecar, data, 0600)
}

func (s *Server) removeSession(id string) {
	part, sidecar := s.sessionPaths(id)
	for _, p := range []string{part, sidecar} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			s.logger.Error("Could not remove upload session file", "path", p, "error", err)
		}
	}
}

// sweepSessions deletes sessions past their TTL. Amortised onto session
// creation rather than given a goroutine: it needs to run rarely, and the
// scheduled purge only exists when --purge-days is set.
func (s *Server) sweepSessions() {
	s.sessionSweepMu.Lock()
	if time.Since(s.lastSessionSweep) < sessionSweepInterval {
		s.sessionSweepMu.Unlock()
		return
	}
	s.lastSessionSweep = time.Now()
	s.sessionSweepMu.Unlock()

	entries, err := os.ReadDir(s.sessionDir())
	if err != nil {
		if !os.IsNotExist(err) {
			s.logger.Error("Could not read upload session directory", "error", err)
		}
		return
	}

	cutoff := time.Now().Add(-sessionTTL)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}

		p := filepath.Join(s.sessionDir(), entry.Name())
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			s.logger.Error("Could not remove stale upload session file", "path", p, "error", err)
		}
	}
}

func (s *Server) sessionURL(r *http.Request, sess *uploadSession) string {
	relative, _ := url.Parse(path.Join(s.proxyPath, "upload", sess.ID, url.PathEscape(sess.Filename)))
	return resolveURL(r, relative, s.proxyPort)
}

// createUploadSessionHandler opens a session. Every limit that can be checked
// up front is checked here, so a client does not discover after transferring
// five gigabytes that its Max-Days header was a typo.
func (s *Server) createUploadSessionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	filename := sanitize(vars["filename"])

	total, err := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
	if err != nil || total < 1 {
		s.metrics.uploadErrors.Add(1)
		http.Error(w, "Upload-Length must be the total size in bytes", http.StatusBadRequest)
		return
	}

	if s.maxUploadSize > 0 && total > s.maxUploadSize {
		s.metrics.uploadErrors.Add(1)
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}

	// The password would have to be spooled next to the data to survive
	// between chunks, and storing it in clear is worse than not offering it.
	if r.Header.Get("X-Encrypt-Password") != "" {
		http.Error(w, "X-Encrypt-Password is not available on resumable uploads — encrypt on the client instead", http.StatusBadRequest)
		return
	}

	// Both quotas are checked against the declared total, so a session that
	// cannot possibly be stored is refused before a byte is transferred.
	if !s.checkStorageQuota(w, total) {
		return
	}

	if !s.checkTempQuota(w, total) {
		return
	}

	contentType := contentTypeForFilename(vars["filename"])

	// Validate the limit headers now; the same values are applied at finalise.
	if _, err := metadataForHeaders(contentType, total, s.randomTokenLength, r.Header); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Invalid upload headers", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.sweepSessions()

	sess := &uploadSession{
		ID:          token(sessionIDLength),
		Filename:    filename,
		ContentType: contentType,
		Total:       total,
		// Recorded now rather than at the finishing chunk: the owner is
		// whoever started the upload, and the last chunk may come from a
		// later run of the client.
		OwnerHash: ownerHashFromHeaders(r.Header),
		CreatedAt: time.Now(),
	}
	sess.MaxDays, _ = strconv.Atoi(r.Header.Get("Max-Days"))
	sess.MaxDownloads, _ = strconv.Atoi(r.Header.Get("Max-Downloads"))

	part, _ := s.sessionPaths(sess.ID)

	if err := s.saveSession(sess); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Could not create upload session", "error", err)
		http.Error(w, "Could not create upload session", http.StatusInternalServerError)
		return
	}

	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) //nolint:gosec // id is server-generated
	if err != nil {
		s.removeSession(sess.ID)
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Could not create upload spool file", "error", err)
		http.Error(w, "Could not create upload session", http.StatusInternalServerError)
		return
	}
	_ = f.Close()

	sessionURL := s.sessionURL(r, sess)

	w.Header().Set("Location", sessionURL)
	w.Header().Set("Cache-Control", "no-store")
	s.setSessionHeaders(w, sess, 0)

	s.logger.Info("Upload session created", "session", maskToken(sess.ID), "filename", filename, "total", total)

	if wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(struct {
			UploadURL string    `json:"upload_url"`
			Offset    int64     `json:"offset"`
			Length    int64     `json:"length"`
			ExpiresAt time.Time `json:"expires_at"`
		}{sessionURL, 0, sess.Total, sess.CreatedAt.Add(sessionTTL)})
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(sessionURL))
}

func (s *Server) setSessionHeaders(w http.ResponseWriter, sess *uploadSession, offset int64) {
	w.Header().Set("Upload-Offset", strconv.FormatInt(offset, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(sess.Total, 10))
	w.Header().Set("Upload-Expires", sess.CreatedAt.Add(sessionTTL).UTC().Format(http.TimeFormat))
}

// openSession resolves and validates the session named by the request, writing
// the error response itself. The filename has to match the one the session was
// created for, so a mistyped URL cannot silently append to another upload.
func (s *Server) openSession(w http.ResponseWriter, r *http.Request) (*uploadSession, bool) {
	vars := mux.Vars(r)

	id := vars["id"]
	if !sessionIDPattern.MatchString(id) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil, false
	}

	sess, err := s.loadSession(id)
	if err != nil || sess.ID != id || sess.Filename != sanitize(vars["filename"]) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil, false
	}

	if time.Since(sess.CreatedAt) > sessionTTL {
		s.removeSession(id)
		http.Error(w, "This upload session has expired", http.StatusGone)
		return nil, false
	}

	return sess, true
}

// headUploadSessionHandler reports how much of the upload the server already
// has, which is all a client needs to resume with no local state beyond the
// session URL.
func (s *Server) headUploadSessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.openSession(w, r)
	if !ok {
		return
	}

	part, _ := s.sessionPaths(sess.ID)

	info, err := os.Stat(part)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	s.setSessionHeaders(w, sess, info.Size())
}

// deleteUploadSessionHandler abandons a session and reclaims its spool file.
func (s *Server) deleteUploadSessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.openSession(w, r)
	if !ok {
		return
	}

	s.lock(sessionLockToken, sess.ID)
	defer s.unlock(sessionLockToken, sess.ID)

	s.removeSession(sess.ID)
	w.WriteHeader(http.StatusNoContent)
}

// patchUploadSessionHandler appends one chunk. A chunk is all-or-nothing: a
// transfer that dies mid-request is truncated back to where it started, so the
// offset reported afterwards is always one a client asked for.
func (s *Server) patchUploadSessionHandler(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.openSession(w, r)
	if !ok {
		return
	}

	start, end, total, err := parseUploadContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if total != sess.Total {
		s.metrics.uploadErrors.Add(1)
		http.Error(w, "Content-Range total does not match the size this session was created for", http.StatusBadRequest)
		return
	}

	s.lock(sessionLockToken, sess.ID)
	defer s.unlock(sessionLockToken, sess.ID)

	part, _ := s.sessionPaths(sess.ID)

	f, err := os.OpenFile(part, os.O_WRONLY, 0600) //nolint:gosec // id is validated against sessionIDPattern
	if err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		s.logger.Error("Could not stat upload spool file", "error", err)
		http.Error(w, "Could not continue upload", http.StatusInternalServerError)
		return
	}

	offset := info.Size()

	// Not an error the client can do anything about except retry from the
	// offset we report, so say what it is rather than just refusing.
	if start != offset {
		_ = f.Close()
		s.setSessionHeaders(w, sess, offset)
		http.Error(w, fmt.Sprintf("This session is at byte %d", offset), http.StatusConflict)
		return
	}

	want := end - start + 1

	// Measured per chunk rather than reserved up front: several sessions can
	// pass the check at creation and only fill the disk as they transfer.
	if !s.checkTempQuota(w, want) {
		_ = f.Close()
		return
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()
		s.logger.Error("Could not seek upload spool file", "error", err)
		http.Error(w, "Could not continue upload", http.StatusInternalServerError)
		return
	}

	written, copyErr := io.Copy(f, io.LimitReader(r.Body, want))

	if copyErr != nil || written != want {
		// Roll the spool file back so the session stays on a boundary the
		// client knows about, whatever arrived before the connection died.
		if truncErr := f.Truncate(offset); truncErr != nil {
			s.logger.Error("Could not roll back a failed chunk", "session", maskToken(sess.ID), "error", truncErr)
		}
		_ = f.Close()

		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Upload chunk incomplete", "session", maskToken(sess.ID), "written", written, "want", want, "error", copyErr)

		// 408 rather than 400: the request was well formed and the body was
		// cut short, which is the one failure a client should simply retry.
		s.setSessionHeaders(w, sess, offset)
		http.Error(w, "Chunk did not arrive in full", http.StatusRequestTimeout)
		return
	}

	if err := f.Close(); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Could not flush upload spool file", "error", err)
		http.Error(w, "Could not continue upload", http.StatusInternalServerError)
		return
	}

	offset += written

	if offset < sess.Total {
		w.Header().Set("Cache-Control", "no-store")
		s.setSessionHeaders(w, sess, offset)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.finishUploadSession(w, r, sess)
}

// finishUploadSession hands the completed spool file to the storage backend and
// answers exactly as a plain PUT would, so a caller can treat the two the same.
func (s *Server) finishUploadSession(w http.ResponseWriter, r *http.Request, sess *uploadSession) {
	part, _ := s.sessionPaths(sess.ID)

	if err := s.prescan(w, part); err != nil {
		// An infected upload is not worth keeping around to be resumed.
		s.removeSession(sess.ID)
		return
	}

	f, err := os.Open(part) //nolint:gosec // id is validated against sessionIDPattern
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Could not read completed upload", "error", err)
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()

	headers := make(http.Header)
	if sess.MaxDays > 0 {
		headers.Set("Max-Days", strconv.Itoa(sess.MaxDays))
	}
	if sess.MaxDownloads > 0 {
		headers.Set("Max-Downloads", strconv.Itoa(sess.MaxDownloads))
	}

	m, err := metadataForHeaders(sess.ContentType, sess.Total, s.randomTokenLength, headers)
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if !s.checkStorageQuota(w, sess.Total) {
		return
	}

	m.OwnerHash = sess.OwnerHash

	uploadToken := token(s.randomTokenLength)

	if err := s.store(r.Context(), uploadToken, sess.Filename, f, sess.ContentType, sess.Total, m, ""); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error storing completed upload", "error", err)
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return
	}

	s.removeSession(sess.ID)

	result := s.newUploadResult(r, uploadToken, sess.Filename, m)
	s.recordOwnership(r, result, m, uploadToken)

	s.writeUploadResponse(w, r, result)
}
