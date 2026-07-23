package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sooua/send.to/server/storage"
)

// uploadResult is the machine-readable description of one stored upload,
// returned when the client sends `Accept: application/json`. The plain-text
// response (a bare URL, delete link in the X-Url-Delete header) is unchanged
// for existing curl users.
type uploadResult struct {
	URL          string     `json:"url"`
	DeleteURL    string     `json:"delete_url"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	ContentType  string     `json:"content_type,omitempty"`
	Encrypted    bool       `json:"encrypted"`
	MaxDownloads *int       `json:"max_downloads,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func (s *Server) newUploadResult(req *http.Request, token, filename string, m metadata) uploadResult {
	escaped := url.PathEscape(filename)
	relativeURL, _ := url.Parse(path.Join(s.proxyPath, token, escaped))
	deleteURL, _ := url.Parse(path.Join(s.proxyPath, token, escaped, m.DeletionToken))

	result := uploadResult{
		URL:         resolveURL(req, relativeURL, s.proxyPort),
		DeleteURL:   resolveURL(req, deleteURL, s.proxyPort),
		Filename:    filename,
		Size:        m.ContentLength,
		ContentType: m.ContentType,
		Encrypted:   m.Encrypted,
	}

	if m.Encrypted {
		result.ContentType = m.DecryptedContentType
	}

	if m.MaxDownloads != -1 {
		maxDownloads := m.MaxDownloads
		result.MaxDownloads = &maxDownloads
	}

	if !m.MaxDate.IsZero() {
		expires := m.MaxDate
		result.ExpiresAt = &expires
	}

	return result
}

func (s *Server) postHandler(w http.ResponseWriter, r *http.Request) {
	if s.maxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadSize)
	}

	if err := r.ParseMultipartForm(_24K); nil != err {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error parsing multipart form", "error", err)
		http.Error(w, "Could not parse multipart form", http.StatusBadRequest)
		return
	}

	token := token(s.randomTokenLength)

	w.Header().Set("Cache-Control", "no-store")

	var results []uploadResult

	for _, fHeaders := range r.MultipartForm.File {
		for _, fHeader := range fHeaders {
			result, err := s.storeMultipartFile(w, r, token, fHeader)
			if err != nil {
				// storeMultipartFile has already written the response.
				return
			}

			w.Header().Add("X-Url-Delete", result.DeleteURL)
			results = append(results, result)
		}
	}

	if wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Files []uploadResult `json:"files"`
		}{Files: results})
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	responseBody := ""
	for _, result := range results {
		responseBody += fmt.Sprintln(result.URL)
	}

	if _, err := w.Write([]byte(responseBody)); err != nil {
		s.logger.Error("Error", "error", err)
	}
}

// storeMultipartFile spools one multipart part to disk, scans it and stores it.
// Extracted from the loop in postHandler so the temp file is cleaned up after
// each part rather than piling up deferred closes until the request finishes.
func (s *Server) storeMultipartFile(w http.ResponseWriter, r *http.Request, token string, fHeader *multipart.FileHeader) (uploadResult, error) {
	var result uploadResult

	filename := sanitize(fHeader.Filename)
	contentType := contentTypeForFilename(fHeader.Filename)

	f, err := fHeader.Open()
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error opening uploaded file", "error", err)
		http.Error(w, "Could not read uploaded file", http.StatusInternalServerError)
		return result, err
	}
	defer storage.CloseCheck(f)

	file, err := os.CreateTemp(s.tempPath, "sendto-upload-")
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not buffer upload", http.StatusInternalServerError)
		return result, err
	}
	defer s.cleanTmpFile(file)

	contentLength, err := io.Copy(file, f)
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not buffer upload", http.StatusInternalServerError)
		return result, err
	}

	if _, err = file.Seek(0, io.SeekStart); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error", "error", err)
		http.Error(w, "Cannot reset cache file", http.StatusInternalServerError)
		return result, err
	}

	if s.maxUploadSize > 0 && contentLength > s.maxUploadSize {
		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Entity too large")
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return result, errors.New("entity too large")
	}

	if err := s.prescan(w, file.Name()); err != nil {
		return result, err
	}

	metadata, err := metadataForRequest(contentType, contentLength, s.randomTokenLength, r)
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Invalid upload headers", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return result, err
	}

	if err := s.store(r.Context(), token, filename, file, contentType, contentLength, metadata, r.Header.Get("X-Encrypt-Password")); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Backend storage error", "error", err)
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return result, err
	}

	return s.newUploadResult(r, token, filename, metadata), nil
}

func (s *Server) putHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	filename := sanitize(vars["filename"])

	contentLength := r.ContentLength

	if s.maxUploadSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadSize)
	}

	defer storage.CloseCheck(r.Body)

	var reader io.Reader = r.Body

	if contentLength < 1 || s.performClamavPrescan {
		file, err := os.CreateTemp(s.tempPath, "sendto-upload-")
		defer s.cleanTmpFile(file)
		if err != nil {
			s.metrics.uploadErrors.Add(1)
			s.logger.Error("Error", "error", err)
			http.Error(w, "Could not buffer upload", http.StatusInternalServerError)
			return
		}

		// queue file to disk, because s3 needs content length
		// and clamav prescan scans a file
		n, err := io.Copy(file, r.Body)
		if err != nil {
			s.metrics.uploadErrors.Add(1)
			s.logger.Error("Error", "error", err)
			http.Error(w, "Could not buffer upload", http.StatusInternalServerError)

			return
		}

		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			s.metrics.uploadErrors.Add(1)
			s.logger.Error("Error", "error", err)
			http.Error(w, "Cannot reset cache file", http.StatusInternalServerError)

			return
		}

		contentLength = n

		if err := s.prescan(w, file.Name()); err != nil {
			return
		}

		reader = file
	}

	if s.maxUploadSize > 0 && contentLength > s.maxUploadSize {
		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Entity too large")
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}

	if contentLength == 0 {
		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Empty content-length")
		http.Error(w, "Could not upload empty file", http.StatusBadRequest)
		return
	}

	contentType := contentTypeForFilename(vars["filename"])

	token := token(s.randomTokenLength)

	metadata, err := metadataForRequest(contentType, contentLength, s.randomTokenLength, r)
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Warn("Invalid upload headers", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store(r.Context(), token, filename, reader, contentType, contentLength, metadata, r.Header.Get("X-Encrypt-Password")); err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error putting new file", "error", err)
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return
	}

	s.writeUploadResponse(w, r, s.newUploadResult(r, token, filename, metadata))
}

// writeUploadResponse answers a completed upload: a bare URL for curl, the full
// record for `Accept: application/json`, and the delete link in a header for
// both. Shared so a resumable upload is indistinguishable from a plain PUT.
func (s *Server) writeUploadResponse(w http.ResponseWriter, r *http.Request, result uploadResult) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Url-Delete", result.DeleteURL)

	if wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(result.URL))
}

// prescan runs the optional ClamAV scan and writes the error response itself,
// so both upload paths reject infected files identically.
func (s *Server) prescan(w http.ResponseWriter, path string) error {
	if !s.performClamavPrescan {
		return nil
	}

	status, err := s.performScan(path)
	if err != nil {
		s.metrics.uploadErrors.Add(1)
		s.logger.Error("Error", "error", err)
		http.Error(w, "Could not perform prescan", http.StatusInternalServerError)
		return err
	}

	if status != clamavScanStatusOK {
		s.metrics.virusScanBlocked.Add(1)
		s.logger.Warn("Clamav prescan positive", "status", status)
		http.Error(w, "Clamav prescan found a virus", http.StatusPreconditionFailed)
		return errors.New("clamav prescan positive")
	}

	return nil
}

// store writes the metadata sidecar and then the payload itself. The
// server-side encryption password is passed explicitly rather than read back
// off the request: a resumable upload finishes on a request that never carried
// one, and a silent mismatch between metadata.Encrypted and the stored bytes
// would leave the file unreadable.
func (s *Server) store(ctx context.Context, token, filename string, reader io.Reader, contentType string, contentLength int64, m metadata, password string) error {
	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(m); err != nil {
		return fmt.Errorf("could not encode metadata: %w", err)
	}

	if err := s.storage.Put(ctx, token, fmt.Sprintf("%s.metadata", filename), buffer, "text/json", uint64(buffer.Len())); err != nil {
		return fmt.Errorf("could not save metadata: %w", err)
	}

	s.logger.Info("Uploading", "token", maskToken(token), "filename", filename, "content_length", contentLength, "content_type", contentType)

	payload, err := attachEncryptionReader(io.NopCloser(reader), password)
	if err != nil {
		return fmt.Errorf("could not encrypt file: %w", err)
	}

	// Encryption changes the byte count (PGP framing plus armor expansion)
	// and the ciphertext length is not known up front. Backends that verify
	// the declared length — Storj aborts the upload on a mismatch — treat 0
	// as "stream until EOF".
	storedLength := uint64(contentLength)
	if m.Encrypted {
		storedLength = 0
	}

	if err := s.storage.Put(ctx, token, filename, payload, contentType, storedLength); err != nil {
		// The metadata sidecar is already written; drop it so the token
		// cannot resolve to a half-created upload.
		if delErr := s.storage.Delete(ctx, token, filename); delErr != nil && !s.storage.IsNotExist(delErr) {
			s.logger.Error("Could not roll back metadata after failed upload", "token", maskToken(token), "filename", filename, "error", delErr)
		}
		return err
	}

	s.metrics.uploads.Add(1)
	s.metrics.uploadBytes.Add(uint64(contentLength))

	return nil
}

func (s *Server) cleanTmpFile(f *os.File) {
	if f != nil {
		err := f.Close()
		if err != nil {
			s.logger.Error("Error closing tmpfile", "error", err, "file", f.Name())
		}

		err = os.Remove(f.Name())
		if err != nil {
			s.logger.Error("Error removing tmpfile", "error", err, "file", f.Name())
		}
	}
}

// fallbackMimeTypes covers extensions that Go's builtin table omits (.txt and
// .bin among them). mime.TypeByExtension only sees the rest through a system
// mime.types file, so on Alpine, scratch or a minimal CI image an uploaded
// notes.txt is stored with an empty Content-Type and later served with no type
// at all — which, combined with the X-Content-Type-Options: nosniff header,
// stops browsers from rendering it.
var fallbackMimeTypes = map[string]string{
	".txt":  "text/plain; charset=utf-8",
	".log":  "text/plain; charset=utf-8",
	".md":   "text/x-markdown",
	".csv":  "text/csv; charset=utf-8",
	".yaml": "application/yaml",
	".yml":  "application/yaml",
	".toml": "application/toml",
	".sh":   "text/x-shellscript; charset=utf-8",
	".zip":  "application/zip",
	".gz":   "application/gzip",
	".tar":  "application/x-tar",
	".7z":   "application/x-7z-compressed",
	".mp3":  "audio/mpeg",
	".mp4":  "video/mp4",
	".webm": "video/webm",
}

// contentTypeForFilename resolves the Content-Type an upload is stored with.
// It never returns an empty string: an unknown extension becomes
// application/octet-stream, which browsers and curl both handle sanely.
func contentTypeForFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))

	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}

	if contentType, ok := fallbackMimeTypes[ext]; ok {
		return contentType
	}

	return "application/octet-stream"
}

// maxDaysLimit caps Max-Days so the resulting deadline cannot overflow
// time.Time. 100 years is far past any legitimate use and keeps the rejection
// message actionable.
const maxDaysLimit = 36500

func metadataForRequest(contentType string, contentLength int64, randomTokenLength int, r *http.Request) (metadata, error) {
	return metadataForHeaders(contentType, contentLength, randomTokenLength, r.Header)
}

// metadataForHeaders builds the stored metadata from the upload's option
// headers. Split out from the request so a resumable upload can apply the
// options it was created with, hours after that request finished.
func metadataForHeaders(contentType string, contentLength int64, randomTokenLength int, h http.Header) (metadata, error) {
	metadata := metadata{
		ContentType:   strings.ToLower(contentType),
		ContentLength: contentLength,
		MaxDate:       time.Time{},
		Downloads:     0,
		MaxDownloads:  -1,
		DeletionToken: token(randomTokenLength) + token(randomTokenLength),
	}

	// A malformed limit used to be discarded silently, so a typo in
	// Max-Downloads produced a link that never expired while the uploader
	// believed it would. Reject it instead.
	if v := h.Get("Max-Downloads"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return metadata, errors.New("Max-Downloads must be a positive integer")
		}
		metadata.MaxDownloads = n
	}

	if v := h.Get("Max-Days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return metadata, errors.New("Max-Days must be a positive integer")
		}
		if n > maxDaysLimit {
			return metadata, fmt.Errorf("Max-Days must be %d or less", maxDaysLimit)
		}
		metadata.MaxDate = time.Now().Add(time.Hour * 24 * time.Duration(n))
	}

	if password := h.Get("X-Encrypt-Password"); password != "" {
		metadata.Encrypted = true
		metadata.ContentType = "text/plain; charset=utf-8"
		metadata.DecryptedContentType = strings.ToLower(contentType)
	} else {
		metadata.Encrypted = false
	}

	return metadata, nil
}
