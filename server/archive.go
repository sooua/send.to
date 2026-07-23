package server

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sooua/send.to/server/storage"
)

// archiveEntry is one validated `token/filename` pair from an archive request.
type archiveEntry struct {
	token    string
	filename string
}

// parseArchiveKeys splits the comma-separated `(tokenA/a.txt,tokenB/b.txt)`
// list into validated entries. Malformed segments are dropped rather than
// indexed blindly — `/(abc).zip` used to panic on the missing slash and was
// only caught by the recovery middleware.
func parseArchiveKeys(files, proxyPath string) []archiveEntry {
	var entries []archiveEntry

	for _, key := range strings.Split(files, ",") {
		key = resolveKey(key, proxyPath)

		token, filename, ok := strings.Cut(key, "/")
		if !ok || token == "" || filename == "" {
			continue
		}

		entries = append(entries, archiveEntry{token: token, filename: sanitize(filename)})
	}

	return entries
}

// copyArchiveEntry streams one stored object into the archive writer. It is a
// method rather than inline code so the reader is closed per entry — the
// previous `defer` inside the loop held every file handle open until the whole
// archive had been written.
func (s *Server) copyArchiveEntry(ctx context.Context, entry archiveEntry, write func(contentLength uint64, r io.Reader) error) error {
	reader, contentLength, err := s.storage.Get(ctx, entry.token, entry.filename, nil)
	defer storage.CloseCheck(reader)

	if err != nil {
		return err
	}

	if err := write(contentLength, reader); err != nil {
		return err
	}

	// Counted only once the bytes are in the archive, matching the plain
	// download path.
	if err := s.increaseDownload(ctx, entry.token, entry.filename); err != nil {
		s.logger.Error("Could not record download", "token", entry.token, "filename", entry.filename, "error", err)
	}

	s.metrics.downloads.Add(1)
	s.metrics.downloadBytes.Add(contentLength)

	return nil
}

// archiveError reports a per-entry failure. Once the first entry has been
// written the response headers and some body bytes are already on the wire, so
// the only possible signal is a truncated archive; the reason goes to the log.
func (s *Server) archiveError(w http.ResponseWriter, entry archiveEntry, err error, headersSent bool) {
	s.metrics.downloadErrors.Add(1)
	s.logger.Error("Error building archive", "token", entry.token, "filename", entry.filename, "error", err)

	if headersSent {
		return
	}

	if s.storage.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	http.Error(w, "Could not retrieve file.", http.StatusInternalServerError)
}

func (s *Server) zipHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	entries := parseArchiveKeys(vars["files"], s.proxyPath)
	if len(entries) == 0 {
		http.Error(w, "No valid files requested", http.StatusBadRequest)
		return
	}

	zipfilename := fmt.Sprintf("sendto-%d.zip", uint16(time.Now().UnixNano()))

	w.Header().Set("Content-Type", "application/zip")
	commonHeader(w, zipfilename)

	zw := zip.NewWriter(w)

	written := 0
	for _, entry := range entries {
		if _, err := s.checkMetadata(r.Context(), entry.token, entry.filename); err != nil {
			s.logger.Error("Error metadata", "error", err)
			continue
		}

		err := s.copyArchiveEntry(r.Context(), entry, func(_ uint64, reader io.Reader) error {
			header := &zip.FileHeader{
				Name:     entry.filename,
				Method:   zip.Store,
				Modified: time.Now().UTC(),
			}

			fw, err := zw.CreateHeader(header)
			if err != nil {
				return err
			}

			_, err = io.Copy(fw, reader)
			return err
		})

		if err != nil {
			s.archiveError(w, entry, err, written > 0)
			return
		}

		written++
	}

	if err := zw.Close(); err != nil {
		s.logger.Error("Error", "error", err)
	}
}

func (s *Server) tarGzHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	entries := parseArchiveKeys(vars["files"], s.proxyPath)
	if len(entries) == 0 {
		http.Error(w, "No valid files requested", http.StatusBadRequest)
		return
	}

	tarfilename := fmt.Sprintf("sendto-%d.tar.gz", uint16(time.Now().UnixNano()))

	w.Header().Set("Content-Type", "application/x-gzip")
	commonHeader(w, tarfilename)

	gw := gzip.NewWriter(w)
	defer storage.CloseCheck(gw)

	zw := tar.NewWriter(gw)
	defer storage.CloseCheck(zw)

	s.writeTarEntries(w, r, zw, entries)
}

func (s *Server) tarHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	entries := parseArchiveKeys(vars["files"], s.proxyPath)
	if len(entries) == 0 {
		http.Error(w, "No valid files requested", http.StatusBadRequest)
		return
	}

	tarfilename := fmt.Sprintf("sendto-%d.tar", uint16(time.Now().UnixNano()))

	w.Header().Set("Content-Type", "application/x-tar")
	commonHeader(w, tarfilename)

	zw := tar.NewWriter(w)
	defer storage.CloseCheck(zw)

	s.writeTarEntries(w, r, zw, entries)
}

// writeTarEntries is shared by the tar and tar.gz handlers, which differ only
// in the writer they wrap.
func (s *Server) writeTarEntries(w http.ResponseWriter, r *http.Request, zw *tar.Writer, entries []archiveEntry) {
	written := 0

	for _, entry := range entries {
		if _, err := s.checkMetadata(r.Context(), entry.token, entry.filename); err != nil {
			s.logger.Error("Error metadata", "error", err)
			continue
		}

		err := s.copyArchiveEntry(r.Context(), entry, func(contentLength uint64, reader io.Reader) error {
			header := &tar.Header{
				Name:    entry.filename,
				Size:    int64(contentLength),
				Mode:    0600,
				ModTime: time.Now().UTC(),
			}

			if err := zw.WriteHeader(header); err != nil {
				return err
			}

			_, err := io.Copy(zw, reader)
			return err
		})

		if err != nil {
			s.archiveError(w, entry, err, written > 0)
			return
		}

		written++
	}
}
