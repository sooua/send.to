package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// Version is the build version, set from cmd at startup.
var Version = "0.0.0"

// metrics holds the counters exposed on /metrics. Plain atomics rather than a
// Prometheus client: the exposition format below is all the scrape needs, and
// it keeps the binary free of another dependency.
type metrics struct {
	uploads          atomic.Uint64
	uploadBytes      atomic.Uint64
	downloads        atomic.Uint64
	downloadBytes    atomic.Uint64
	deletes          atomic.Uint64
	expiredPurged    atomic.Uint64
	rateLimited      atomic.Uint64
	uploadErrors     atomic.Uint64
	downloadErrors   atomic.Uint64
	virusScanBlocked atomic.Uint64
}

// healthHandler reports liveness plus the details an operator actually needs
// when a probe goes red: which build is running and which backend it talks to.
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	body := struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Storage string `json:"storage"`
		Uptime  string `json:"uptime"`
	}{
		Status:  "ok",
		Version: Version,
		Uptime:  time.Since(s.startedAt).Truncate(time.Second).String(),
	}

	if s.storage != nil {
		body.Storage = s.storage.Type()
	}

	// Text clients (curl, the original health probe) keep the old plain-text
	// behaviour; anything asking for JSON gets the structured form.
	if !wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "ok version=%s storage=%s uptime=%s\n", body.Version, body.Storage, body.Uptime)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// metricsHandler exposes counters in Prometheus text exposition format.
func (s *Server) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	counters := []struct {
		name string
		help string
		val  uint64
	}{
		{"sendto_uploads_total", "Uploads stored successfully.", s.metrics.uploads.Load()},
		{"sendto_upload_bytes_total", "Bytes accepted from uploads.", s.metrics.uploadBytes.Load()},
		{"sendto_upload_errors_total", "Uploads that failed.", s.metrics.uploadErrors.Load()},
		{"sendto_downloads_total", "Downloads served to completion.", s.metrics.downloads.Load()},
		{"sendto_download_bytes_total", "Bytes written to download clients.", s.metrics.downloadBytes.Load()},
		{"sendto_download_errors_total", "Downloads that failed.", s.metrics.downloadErrors.Load()},
		{"sendto_deletes_total", "Uploads deleted via deletion token.", s.metrics.deletes.Load()},
		{"sendto_expired_purged_total", "Uploads removed after exhausting their download or day limit.", s.metrics.expiredPurged.Load()},
		{"sendto_rate_limited_total", "Requests rejected with 429.", s.metrics.rateLimited.Load()},
		{"sendto_virus_blocked_total", "Uploads rejected by the ClamAV prescan.", s.metrics.virusScanBlocked.Load()},
	}

	for _, c := range counters {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", c.name, c.help, c.name, c.name, c.val)
	}

	_, _ = fmt.Fprintf(w,
		"# HELP sendto_uptime_seconds Seconds since process start.\n"+
			"# TYPE sendto_uptime_seconds gauge\nsendto_uptime_seconds %d\n",
		int64(time.Since(s.startedAt).Seconds()))

	// Disk is the resource an operator has to watch, and a 507 only arrives
	// once it is already too late to add more.
	gauges := []struct {
		name string
		help string
		val  int64
	}{
		{"sendto_storage_used_bytes", "Bytes held by the storage backend, as counted since startup.", s.quota.usage()},
		{"sendto_storage_limit_bytes", "Total storage limit, 0 when unlimited.", s.maxStorageSize},
		{"sendto_temp_used_bytes", "Bytes of spool files for uploads in progress.", s.tempUsage()},
		{"sendto_temp_limit_bytes", "Spool space limit, 0 when unlimited.", s.maxTempSize},
	}

	for _, g := range gauges {
		_, _ = fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", g.name, g.help, g.name, g.name, g.val)
	}
}

// qrHandler renders a QR code for a share link. The web UI has no QR library
// of its own; reusing the encoder already vendored for the preview pages keeps
// the frontend bundle unchanged.
func (s *Server) qrHandler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	// Only ever encode links that point back at this instance, so the
	// endpoint cannot be used to mint QR codes for arbitrary third-party
	// URLs under the operator's domain.
	if !s.isOwnURL(r, target) {
		http.Error(w, "url must point at this server", http.StatusBadRequest)
		return
	}

	size := 256
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 64 && n <= 1024 {
			size = n
		}
	}

	png, err := qrcode.Encode(target, qrcode.Medium, size)
	if err != nil {
		s.logger.Error("Error encoding QR code", "error", err)
		http.Error(w, "could not render QR code", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	_, _ = w.Write(png)
}

// wantsJSON reports whether the client asked for an application/json response.
func wantsJSON(hdr http.Header) bool {
	return acceptsMediaType(hdr, "application/json")
}
