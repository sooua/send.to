/*
The MIT License (MIT)

Copyright (c) 2025 sooua

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/tg123/go-htpasswd"
	"github.com/tomasen/realip"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/time/rate"

	"github.com/sooua/send.to/server/storage"
)

// parse request with maximum memory of _24Kilobits
const _24K = (1 << 3) * 24

// parse request with maximum memory of _5Megabytes
const _5M = (1 << 20) * 5

// OptionFn is the option function type
type OptionFn func(*Server)

// ClamavHost sets clamav host
func ClamavHost(s string) OptionFn {
	return func(srvr *Server) {
		srvr.ClamAVDaemonHost = s
	}
}

// PerformClamavPrescan enables clamav prescan on upload
func PerformClamavPrescan(b bool) OptionFn {
	return func(srvr *Server) {
		srvr.performClamavPrescan = b
	}
}

// VirustotalKey sets virus total key
func VirustotalKey(s string) OptionFn {
	return func(srvr *Server) {
		srvr.VirusTotalKey = s
	}
}

// Listener set listener
func Listener(s string) OptionFn {
	return func(srvr *Server) {
		srvr.ListenerString = s
	}

}

// CorsDomains sets CORS domains
func CorsDomains(s string) OptionFn {
	return func(srvr *Server) {
		srvr.CorsDomains = s
	}

}

// EmailContact sets email contact
func EmailContact(emailContact string) OptionFn {
	return func(srvr *Server) {
		srvr.emailContact = emailContact
	}
}

// GoogleAnalytics sets GA key
func GoogleAnalytics(gaKey string) OptionFn {
	return func(srvr *Server) {
		srvr.gaKey = gaKey
	}
}

// UserVoice sets UV key
func UserVoice(userVoiceKey string) OptionFn {
	return func(srvr *Server) {
		srvr.userVoiceKey = userVoiceKey
	}
}

// TLSListener sets TLS listener and option
func TLSListener(s string, t bool) OptionFn {
	return func(srvr *Server) {
		srvr.TLSListenerString = s
		srvr.TLSListenerOnly = t
	}

}

// ProfileListener sets profile listener
func ProfileListener(s string) OptionFn {
	return func(srvr *Server) {
		srvr.ProfileListenerString = s
	}
}

// WebPath sets web path
func WebPath(s string) OptionFn {
	return func(srvr *Server) {
		if s[len(s)-1:] != "/" {
			s = filepath.Join(s, "")
		}

		srvr.webPath = s
	}
}

// ProxyPath sets proxy path
func ProxyPath(s string) OptionFn {
	return func(srvr *Server) {
		if s[len(s)-1:] != "/" {
			s = filepath.Join(s, "")
		}

		srvr.proxyPath = s
	}
}

// ProxyPort sets proxy port
func ProxyPort(s string) OptionFn {
	return func(srvr *Server) {
		srvr.proxyPort = s
	}
}

// TempPath sets temp path
func TempPath(s string) OptionFn {
	return func(srvr *Server) {
		if s[len(s)-1:] != "/" {
			s = filepath.Join(s, "")
		}

		srvr.tempPath = s
	}
}

// LogFile sets log file
func LogFile(s string) OptionFn {
	return func(srvr *Server) {
		f, err := os.OpenFile(s, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			slog.Error("Error opening log file", "error", err)
			os.Exit(1)
		}

		srvr.logger = slog.New(slog.NewJSONHandler(f, nil))
	}
}

// Logger sets logger
func Logger(logger *slog.Logger) OptionFn {
	return func(srvr *Server) {
		srvr.logger = logger
	}
}

// MaxUploadSize sets max upload size
func MaxUploadSize(kbytes int64) OptionFn {
	return func(srvr *Server) {
		srvr.maxUploadSize = kbytes * 1024
	}

}

// RateLimit set rate limit
func RateLimit(requests int) OptionFn {
	return func(srvr *Server) {
		srvr.rateLimitRequests = requests
	}
}

// ShutdownTimeout sets how long Run() waits for in-flight requests to
// finish during graceful shutdown. Zero or negative falls back to 30s.
func ShutdownTimeout(d time.Duration) OptionFn {
	return func(srvr *Server) {
		srvr.shutdownTimeout = d
	}
}

// RandomTokenLength sets random token length
func RandomTokenLength(length int) OptionFn {
	return func(srvr *Server) {
		srvr.randomTokenLength = length
	}
}

// Purge sets purge days and option
func Purge(days, interval int) OptionFn {
	return func(srvr *Server) {
		srvr.purgeDays = time.Duration(days) * time.Hour * 24
		srvr.purgeInterval = time.Duration(interval) * time.Hour
	}
}

// ForceHTTPS sets forcing https
func ForceHTTPS() OptionFn {
	return func(srvr *Server) {
		srvr.forceHTTPS = true
	}
}

// EnableProfiler sets enable profiler
func EnableProfiler() OptionFn {
	return func(srvr *Server) {
		srvr.profilerEnabled = true
	}
}

// UseStorage set storage to use
func UseStorage(s storage.Storage) OptionFn {
	return func(srvr *Server) {
		srvr.storage = s
	}
}

// UseLetsEncrypt set letsencrypt usage
func UseLetsEncrypt(hosts []string) OptionFn {
	return func(srvr *Server) {
		cacheDir := "./cache/"

		m := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			Cache:  autocert.DirCache(cacheDir),
			HostPolicy: func(_ context.Context, host string) error {
				found := false

				for _, h := range hosts {
					found = found || strings.HasSuffix(host, h)
				}

				if !found {
					return errors.New("acme/autocert: host not configured")
				}

				return nil
			},
		}

		srvr.tlsConfig = m.TLSConfig()
		srvr.tlsConfig.GetCertificate = m.GetCertificate
	}
}

// TLSConfig sets TLS config
func TLSConfig(cert, pk string) OptionFn {
	certificate, err := tls.LoadX509KeyPair(cert, pk)
	return func(srvr *Server) {
		srvr.tlsConfig = &tls.Config{
			GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				return &certificate, err
			},
		}
	}
}

// HTTPAuthCredentials sets basic http auth credentials
func HTTPAuthCredentials(user string, pass string) OptionFn {
	return func(srvr *Server) {
		srvr.authUser = user
		srvr.authPass = pass
	}
}

// HTTPAuthHtpasswd sets basic http auth htpasswd file
func HTTPAuthHtpasswd(htpasswdPath string) OptionFn {
	return func(srvr *Server) {
		srvr.authHtpasswd = htpasswdPath
	}
}

// HTTPAUTHFilterOptions sets basic http auth ips whitelist
func HTTPAUTHFilterOptions(options IPFilterOptions) OptionFn {
	for i, allowedIP := range options.AllowedIPs {
		options.AllowedIPs[i] = strings.TrimSpace(allowedIP)
	}

	return func(srvr *Server) {
		srvr.authIPFilterOptions = &options
	}
}

// FilterOptions sets ip filtering
func FilterOptions(options IPFilterOptions) OptionFn {
	for i, allowedIP := range options.AllowedIPs {
		options.AllowedIPs[i] = strings.TrimSpace(allowedIP)
	}

	for i, blockedIP := range options.BlockedIPs {
		options.BlockedIPs[i] = strings.TrimSpace(blockedIP)
	}

	return func(srvr *Server) {
		srvr.ipFilterOptions = &options
	}
}

// Server is the main application
type Server struct {
	authUser            string
	authPass            string
	authHtpasswd        string
	authIPFilterOptions *IPFilterOptions

	htpasswdFile *htpasswd.File
	authIPFilter *ipFilter

	logger *slog.Logger

	tlsConfig *tls.Config

	profilerEnabled bool

	locksMu sync.Mutex
	locks   map[string]*fileLock

	sessionSweepMu   sync.Mutex
	lastSessionSweep time.Time

	maxUploadSize     int64
	rateLimitRequests int
	rateLimiter       *ipRateLimiter

	shutdownTimeout time.Duration

	purgeDays     time.Duration
	purgeInterval time.Duration

	storage storage.Storage

	forceHTTPS bool

	randomTokenLength int

	ipFilterOptions *IPFilterOptions

	VirusTotalKey        string
	ClamAVDaemonHost     string
	performClamavPrescan bool

	tempPath string

	webPath      string
	proxyPath    string
	proxyPort    string
	emailContact string
	gaKey        string
	userVoiceKey string

	startedAt time.Time
	metrics   metrics

	TLSListenerOnly bool

	CorsDomains           string
	ListenerString        string
	TLSListenerString     string
	ProfileListenerString string

	Certificate string

	LetsEncryptCache string
}

// New is the factory fot Server
func New(options ...OptionFn) (*Server, error) {
	s := &Server{
		locks:     make(map[string]*fileLock),
		startedAt: time.Now(),
	}

	for _, optionFn := range options {
		optionFn(s)
	}

	return s, nil
}

// panicHandler wraps an http.Handler with panic recovery middleware.
// If the wrapped handler panics, it recovers and returns a 500 Internal Server Error.
func panicHandler(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Panic recovered", "error", err, "method", r.Method, "path", redactPath(r.URL.Path))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logHandler wraps an http.Handler with HTTP request logging middleware.
func logHandler(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)
		logger.Info("HTTP request",
			"method", r.Method,
			// Share and deletion tokens are bearer secrets; never log them raw.
			"path", redactPath(r.URL.Path),
			"status", lw.statusCode,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// loggingResponseWriter captures the status code written by the handler.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}

// rateLimiterIdleTTL is how long an idle per-IP bucket is kept before it is
// evicted. It must be long enough for a full bucket to refill, otherwise
// eviction would hand an abusive IP a fresh burst allowance.
const rateLimiterIdleTTL = 10 * time.Minute

// ipRateLimiter provides per-IP rate limiting using token buckets.
//
// Entries are evicted after rateLimiterIdleTTL of inactivity; without that the
// map grows once per distinct source address seen and never shrinks, which a
// single port scan is enough to turn into unbounded memory growth.
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiterEntry
	rate     rate.Limit
	burst    int
	lastGC   time.Time
}

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(requests int, window time.Duration) *ipRateLimiter {
	// rate.Limit is events per second.
	return &ipRateLimiter{
		limiters: make(map[string]*ipLimiterEntry),
		rate:     rate.Limit(float64(requests) / window.Seconds()),
		burst:    requests,
		lastGC:   time.Now(),
	}
}

func (rl *ipRateLimiter) allow(ip string) bool {
	now := time.Now()

	rl.mu.Lock()
	entry, ok := rl.limiters[ip]
	if !ok {
		entry = &ipLimiterEntry{limiter: rate.NewLimiter(rl.rate, rl.burst)}
		rl.limiters[ip] = entry
	}
	entry.lastSeen = now

	// Amortised sweep: at most once per TTL, on whichever request happens
	// to arrive first. Cheap enough to do inline and needs no goroutine.
	if now.Sub(rl.lastGC) > rateLimiterIdleTTL {
		for key, e := range rl.limiters {
			if now.Sub(e.lastSeen) > rateLimiterIdleTTL {
				delete(rl.limiters, key)
			}
		}
		rl.lastGC = now
	}
	limiter := entry.limiter
	rl.mu.Unlock()

	return limiter.Allow()
}

// rateLimit wraps a handler with per-IP rate limiting against the server's
// single shared limiter. Every rate-limited route draws from the same budget,
// so `--rate-limit 60` means 60 requests per minute per IP in total rather
// than 60 per route.
func (s *Server) rateLimit(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		// realip only trusts X-Forwarded-For/X-Real-IP from private and
		// loopback peers, so a direct client cannot spoof its way into a
		// fresh bucket by setting the header itself.
		ip := realip.FromRequest(r)

		if !s.rateLimiter.allow(ip) {
			s.logger.Warn("Rate limit exceeded", "ip", ip, "path", redactPath(r.URL.Path))
			s.metrics.rateLimited.Add(1)
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	}
}

// Run starts Server
func (s *Server) Run() {
	listening := false

	var servers []*http.Server

	if s.rateLimitRequests > 0 {
		s.rateLimiter = newIPRateLimiter(s.rateLimitRequests, time.Minute)
	}

	// Uploads without a Content-Length, and every upload when the ClamAV
	// prescan is on, are spooled here first. Creating it up front turns a
	// per-request "no such file or directory" into a startup failure.
	if s.tempPath != "" {
		if err := os.MkdirAll(s.tempPath, 0700); err != nil {
			s.logger.Error("Could not create temp path", "path", s.tempPath, "error", err)
			os.Exit(1)
		}
	}

	if s.profilerEnabled {
		listening = true

		profileAddr := s.ProfileListenerString
		if profileAddr == "" {
			profileAddr = "127.0.0.1:6060"
		}

		pprofSrv := &http.Server{
			Addr:              profileAddr,
			ReadHeaderTimeout: 10 * time.Second,
		}
		servers = append(servers, pprofSrv)

		go func() {
			s.logger.Info("Profiler listening", "addr", profileAddr)
			if err := pprofSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.logger.Error("Profiler server error", "error", err)
			}
		}()
	}

	r := mux.NewRouter()

	var fs http.FileSystem

	if s.webPath != "" {
		s.logger.Info("Using static file path", "path", s.webPath)

		fs = http.Dir(s.webPath)

		htmlTemplates, _ = htmlTemplates.ParseGlob(filepath.Join(s.webPath, "*.html"))
		textTemplates, _ = textTemplates.ParseGlob(filepath.Join(s.webPath, "*.txt"))
	} else {
		s.logger.Info("No web-path set, serving API only")
		// Create a temporary empty directory for static file serving
		tmpDir, _ := os.MkdirTemp("", "sendto-web-*")
		fs = http.Dir(tmpDir)
	}

	staticHandler := http.FileServer(fs)

	r.PathPrefix("/images/").Handler(staticHandler).Methods("GET")
	r.PathPrefix("/styles/").Handler(staticHandler).Methods("GET")
	r.PathPrefix("/scripts/").Handler(staticHandler).Methods("GET")
	r.PathPrefix("/fonts/").Handler(staticHandler).Methods("GET")
	r.PathPrefix("/ico/").Handler(staticHandler).Methods("GET")
	r.PathPrefix("/_astro/").Handler(staticHandler).Methods("GET")

	// Astro static pages: serve index.html from each directory
	if s.webPath != "" {
		for _, page := range []string{"about", "api-docs", "use-cases", "preview"} {
			pagePath := filepath.Join(s.webPath, page, "index.html")
			r.HandleFunc("/"+page, func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, pagePath)
			}).Methods("GET")
			r.HandleFunc("/"+page+"/", func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, pagePath)
			}).Methods("GET")
		}
	}
	r.HandleFunc("/favicon.ico", staticHandler.ServeHTTP).Methods("GET")
	r.HandleFunc("/favicon.svg", staticHandler.ServeHTTP).Methods("GET")
	r.HandleFunc("/robots.txt", staticHandler.ServeHTTP).Methods("GET")

	r.HandleFunc("/{filename:(?:favicon\\.ico|robots\\.txt|health\\.html)}", s.basicAuthHandler(http.HandlerFunc(s.putHandler))).Methods("PUT")

	// The upload list is reachable only by presenting the owner token, so it
	// is a read of the caller's own data rather than a public listing.
	r.HandleFunc("/owner/files", s.basicAuthHandler(s.rateLimit(http.HandlerFunc(s.ownerFilesHandler)))).Methods("GET")

	r.HandleFunc("/health.html", s.healthHandler).Methods("GET")
	r.HandleFunc("/health", s.healthHandler).Methods("GET")
	r.HandleFunc("/metrics", s.metricsHandler).Methods("GET")
	r.HandleFunc("/qr", s.rateLimit(http.HandlerFunc(s.qrHandler))).Methods("GET")
	r.HandleFunc("/", s.viewHandler).Methods("GET")

	// Archive routes stream an arbitrary number of stored objects in a
	// single request, so they get the same rate limit as plain downloads.
	r.HandleFunc("/({files:.*}).zip", s.rateLimit(http.HandlerFunc(s.zipHandler))).Methods("GET")
	r.HandleFunc("/({files:.*}).tar", s.rateLimit(http.HandlerFunc(s.tarHandler))).Methods("GET")
	r.HandleFunc("/({files:.*}).tar.gz", s.rateLimit(http.HandlerFunc(s.tarGzHandler))).Methods("GET")

	// Collections. The archive form is registered first so `.zip` is not read
	// as part of the token.
	r.HandleFunc("/c/{token:[0-9a-zA-Z]+}.{format:(?:zip|tar|tar\\.gz)}", s.rateLimit(http.HandlerFunc(s.collectionArchiveHandler))).Methods("GET")
	r.HandleFunc("/c/{token:[0-9a-zA-Z]+}", s.rateLimit(http.HandlerFunc(s.collectionHandler))).Methods("GET")
	r.HandleFunc("/c/{token:[0-9a-zA-Z]+}/{deletionToken}", s.deleteCollectionHandler).Methods("DELETE")
	r.HandleFunc("/collection", s.basicAuthHandler(s.rateLimit(http.HandlerFunc(s.createCollectionHandler)))).Methods("POST")

	// Resumable uploads. Registered ahead of the download and deletion routes
	// because `/upload/{id}/{filename}` and `/{token}/{filename}/{deleteToken}`
	// have the same shape; first match wins, and an upload token can never
	// collide with the literal "upload" at any sane --random-token-length.
	r.HandleFunc("/upload/{filename}", s.basicAuthHandler(s.rateLimit(http.HandlerFunc(s.createUploadSessionHandler)))).Methods("POST")
	r.HandleFunc("/upload/{id}/{filename}", s.basicAuthHandler(s.rateLimit(http.HandlerFunc(s.patchUploadSessionHandler)))).Methods("PATCH")
	r.HandleFunc("/upload/{id}/{filename}", s.basicAuthHandler(http.HandlerFunc(s.headUploadSessionHandler))).Methods("HEAD")
	r.HandleFunc("/upload/{id}/{filename}", s.basicAuthHandler(http.HandlerFunc(s.deleteUploadSessionHandler))).Methods("DELETE")

	r.HandleFunc("/{token}/{filename}", s.headHandler).Methods("HEAD")
	r.HandleFunc("/{action:(?:download|get|inline)}/{token}/{filename}", s.headHandler).Methods("HEAD")

	r.HandleFunc("/{token}/{filename}", s.previewHandler).MatcherFunc(func(r *http.Request, rm *mux.RouteMatch) (match bool) {
		// The file will show a preview page when opening the link in browser directly or
		// from external link. If the referer url path and current path are the same it will be
		// downloaded.
		if !acceptsHTML(r.Header) {
			return false
		}

		match = r.Referer() == ""

		if u, err := url.Parse(r.Referer()); err != nil {
			s.logger.Error("Error parsing referer", "error", err)
		} else {
			match = match || (u.Path != r.URL.Path)
		}
		return
	}).Methods("GET")

	getHandlerFn := s.rateLimit(http.HandlerFunc(s.getHandler))

	r.HandleFunc("/{token}/{filename}", getHandlerFn).Methods("GET")
	r.HandleFunc("/{action:(?:download|get|inline)}/{token}/{filename}", getHandlerFn).Methods("GET")

	putHandlerFn := s.rateLimit(http.HandlerFunc(s.putHandler))
	postHandlerFn := s.rateLimit(http.HandlerFunc(s.postHandler))

	// Scanning proxies an arbitrary request body to ClamAV / VirusTotal and
	// spools it to disk on the way. Left open they let anyone burn the
	// operator's VirusTotal quota and fill TEMP_PATH, so they are treated as
	// write endpoints: same auth and same rate limit as an upload.
	r.HandleFunc("/{filename}/virustotal", s.basicAuthHandler(s.rateLimit(http.HandlerFunc(s.virusTotalHandler)))).Methods("PUT")
	r.HandleFunc("/{filename}/scan", s.basicAuthHandler(s.rateLimit(http.HandlerFunc(s.scanHandler)))).Methods("PUT")
	r.HandleFunc("/put/{filename}", s.basicAuthHandler(putHandlerFn)).Methods("PUT")
	r.HandleFunc("/upload/{filename}", s.basicAuthHandler(putHandlerFn)).Methods("PUT")
	r.HandleFunc("/{filename}", s.basicAuthHandler(putHandlerFn)).Methods("PUT")
	r.HandleFunc("/", s.basicAuthHandler(postHandlerFn)).Methods("POST")
	// r.HandleFunc("/{page}", viewHandler).Methods("GET")

	r.HandleFunc("/{token}/{filename}/{deletionToken}", s.deleteHandler).Methods("DELETE")

	r.NotFoundHandler = http.HandlerFunc(s.notFoundHandler)

	_ = mime.AddExtensionType(".md", "text/x-markdown")

	s.logger.Info("send.to server started", "temp_folder", s.tempPath, "storage_provider", s.storage.Type())

	var allowedOrigins map[string]bool
	if len(s.CorsDomains) > 0 {
		allowedOrigins = make(map[string]bool)
		for _, origin := range strings.Split(s.CorsDomains, ",") {
			allowedOrigins[strings.TrimSpace(origin)] = true
		}
	}
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowedOrigins == nil || allowedOrigins[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Range, Max-Downloads, Max-Days, Upload-Length, X-Owner-Token, X-Encrypt-Password, X-Decrypt-Password, Authorization")
					w.Header().Set("Access-Control-Expose-Headers", "X-Url-Delete, Location, Upload-Offset, Upload-Length, Upload-Expires")
					w.Header().Set("Access-Control-Max-Age", "86400")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	h := corsMiddleware(
		panicHandler(
			ipFilterHandler(
				logHandler(
					securityHeadersHandler(
						LoveHandler(
							s.RedirectHandler(r))),
					s.logger,
				),
				s.ipFilterOptions,
			),
			s.logger,
		),
	)

	if !s.TLSListenerOnly {
		listening = true
		s.logger.Info("Starting to listen", "addr", s.ListenerString)

		srvr := &http.Server{
			Addr:              s.ListenerString,
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      300 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
		servers = append(servers, srvr)

		go func() {
			if err := srvr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.logger.Error("HTTP server error", "error", err)
				os.Exit(1)
			}
		}()
	}

	if s.TLSListenerString != "" {
		listening = true
		s.logger.Info("Starting to listen for TLS", "addr", s.TLSListenerString)

		srvr := &http.Server{
			Addr:              s.TLSListenerString,
			Handler:           h,
			TLSConfig:         s.tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      300 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
		servers = append(servers, srvr)

		go func() {
			if err := srvr.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.logger.Error("TLS server error", "error", err)
				os.Exit(1)
			}
		}()
	}

	s.logger.Info("----------------------------")

	if s.purgeDays > 0 {
		go s.purgeHandler()
	}

	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt)
	signal.Notify(term, syscall.SIGTERM)

	if listening {
		<-term
	} else {
		s.logger.Info("No listener active")
	}

	timeout := s.shutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	s.logger.Info("Shutting down server", "timeout", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, srvr := range servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			if err := srv.Shutdown(ctx); err != nil {
				s.logger.Error("Graceful shutdown failed; forcing close", "addr", srv.Addr, "error", err)
				_ = srv.Close()
			}
		}(srvr)
	}
	wg.Wait()

	s.logger.Info("Server stopped")
}
