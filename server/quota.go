package server

// Two limits on how much disk an instance can be made to consume.
//
// MAX_UPLOAD_SIZE bounds one file, which is no bound at all: a public instance
// can be filled by uploading a permitted file often enough. Resumable uploads
// made that sharper — an unfinished session is deliberately kept for 24 hours,
// so a client can open sessions and simply stop, and the bytes stay.
//
//	MAX_STORAGE_SIZE  total bytes of stored uploads
//	MAX_TEMP_SIZE     total bytes of spool files under TEMP_PATH
//
// Both answer 507 when exceeded. The first is a running counter seeded from the
// backend at startup; the second is measured from the directory each time,
// because it is small, changes constantly, and is the one an attacker aims at.

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// storageQuota tracks how much the storage backend holds.
//
// It is a counter rather than a measurement because measuring means walking the
// bucket: fine once at startup, absurd once per upload. That makes it an
// estimate — it drifts if files are removed behind the server's back, and each
// replica of a multi-instance deployment counts only its own writes — and a
// restart re-seeds it from the backend, which is the correction.
type storageQuota struct {
	limit int64
	used  atomic.Int64
}

func (q *storageQuota) enabled() bool {
	return q != nil && q.limit > 0
}

// allows reports whether n more bytes fit.
func (q *storageQuota) allows(n int64) bool {
	if !q.enabled() {
		return true
	}
	return q.used.Load()+n <= q.limit
}

func (q *storageQuota) add(n int64) {
	if q.enabled() && n > 0 {
		q.used.Add(n)
	}
}

// sub never goes below zero: the counter is an estimate, and a negative one
// would hand out free space after enough drift.
func (q *storageQuota) sub(n int64) {
	if !q.enabled() || n <= 0 {
		return
	}

	for {
		current := q.used.Load()
		next := current - n
		if next < 0 {
			next = 0
		}
		if q.used.CompareAndSwap(current, next) {
			return
		}
	}
}

func (q *storageQuota) usage() int64 {
	if q == nil {
		return 0
	}
	return q.used.Load()
}

// initStorageQuota seeds the counter from the backend. A backend that cannot
// count what it holds is a hard failure rather than a warning: an operator who
// asked for a limit must not be left believing one is in force.
func (s *Server) initStorageQuota() error {
	if s.maxStorageSize <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	used, err := s.storage.Usage(ctx)
	if err != nil {
		return err
	}

	s.quota = &storageQuota{limit: s.maxStorageSize}
	s.quota.add(int64(used))

	s.logger.Info("Total storage quota in force",
		"limit", formatSize(s.maxStorageSize),
		"used", formatSize(int64(used)),
		"storage_provider", s.storage.Type())

	return nil
}

// reseedStorageQuota re-measures the backend and resets the counter to it.
// Called after a purge sweep, which is the one moment the counter is known to
// be wrong by an unknown amount. A failure only leaves the old estimate in
// place, so it is logged rather than fatal — unlike at startup, where an
// operator is still owed a hard answer about whether the limit works at all.
func (s *Server) reseedStorageQuota(ctx context.Context) {
	if !s.quota.enabled() {
		return
	}

	used, err := s.storage.Usage(ctx)
	if err != nil {
		s.logger.Error("Could not re-measure storage after purge", "error", err)
		return
	}

	s.quota.used.Store(int64(used))
}

// checkStorageQuota writes the 507 itself and reports whether the upload may
// proceed, so every upload path refuses identically.
func (s *Server) checkStorageQuota(w http.ResponseWriter, contentLength int64) bool {
	if s.quota.allows(contentLength) {
		return true
	}

	s.metrics.uploadErrors.Add(1)
	s.logger.Warn("Storage quota exhausted",
		"limit", s.maxStorageSize, "used", s.quota.usage(), "requested", contentLength)
	http.Error(w, "This server is full", http.StatusInsufficientStorage)

	return false
}

// tempUsage adds up the spool files: partly finished upload sessions, and the
// temporary files a length-less PUT or a ClamAV prescan writes. Measured rather
// than counted because a spool file is created and removed constantly, and a
// counter that drifts here drifts towards refusing every upload.
func (s *Server) tempUsage() int64 {
	var total int64

	for _, dir := range []string{s.tempDir(), s.sessionDir()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if info, err := entry.Info(); err == nil {
				total += info.Size()
			}
		}
	}

	return total
}

// tempDir is where os.CreateTemp writes when TEMP_PATH is unset.
func (s *Server) tempDir() string {
	if s.tempPath == "" {
		return os.TempDir()
	}
	return filepath.Clean(s.tempPath)
}

// checkTempQuota reports whether n more bytes of spool space may be used,
// writing the 507 itself.
func (s *Server) checkTempQuota(w http.ResponseWriter, n int64) bool {
	if s.maxTempSize <= 0 {
		return true
	}

	// A PUT without a Content-Length asks with -1: there is nothing to reserve,
	// only "is there room for anything at all".
	if n < 0 {
		n = 0
	}

	if used := s.tempUsage(); used+n <= s.maxTempSize {
		return true
	}

	s.metrics.uploadErrors.Add(1)
	s.logger.Warn("Temporary space exhausted", "limit", s.maxTempSize, "requested", n)
	http.Error(w, "This server has no room for another upload in progress", http.StatusInsufficientStorage)

	return false
}
