package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/tg123/go-htpasswd"
	"github.com/tomasen/realip"
)

// RedirectHandler handles redirect
func (s *Server) RedirectHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.forceHTTPS {
			// we don't want to enforce https
		} else if r.URL.Path == "/health.html" {
			// health check url won't redirect
		} else if strings.HasSuffix(ipAddrFromRemoteAddr(r.Host), ".onion") {
			// .onion addresses cannot get a valid certificate, so don't redirect
		} else if r.Header.Get("X-Forwarded-Proto") == "https" {
		} else if r.TLS != nil {
		} else {
			u := getURL(r, s.proxyPort)
			u.Scheme = "https"
			if len(s.proxyPort) == 0 && len(s.TLSListenerString) > 0 {
				_, port, err := net.SplitHostPort(s.TLSListenerString)
				if err != nil || port == "443" {
					port = ""
				}

				if len(port) > 0 {
					u.Host = net.JoinHostPort(u.Hostname(), port)
				} else {
					u.Host = u.Hostname()
				}
			}

			http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
			return
		}

		h.ServeHTTP(w, r)
	}
}

// LoveHandler Create a log handler for every request it receives.
func LoveHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-made-with", "<3 by sooua")
		w.Header().Set("x-served-by", "Proudly served by send.to")
		w.Header().Set("server", "send.to HTTP Server")
		h.ServeHTTP(w, r)
	}
}

func ipFilterHandler(h http.Handler, ipFilterOptions *IPFilterOptions) http.HandlerFunc {
	if ipFilterOptions == nil {
		return h.ServeHTTP
	}
	wrapped := WrapIPFilter(h, ipFilterOptions)
	return wrapped.ServeHTTP
}

func (s *Server) basicAuthHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authUser == "" && s.authPass == "" && s.authHtpasswd == "" {
			h.ServeHTTP(w, r)
			return
		}

		if s.htpasswdFile == nil && s.authHtpasswd != "" {
			htpasswdFile, err := htpasswd.New(s.authHtpasswd, htpasswd.DefaultSystems, nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			s.htpasswdFile = htpasswdFile
		}

		if s.authIPFilter == nil && s.authIPFilterOptions != nil {
			s.authIPFilter = newIPFilter(s.authIPFilterOptions)
		}

		w.Header().Set("WWW-Authenticate", "Basic realm=\"Restricted\"")

		var authorized bool
		if s.authIPFilter != nil {
			remoteIP := realip.FromRequest(r)
			authorized = s.authIPFilter.Allowed(remoteIP)
		}

		username, password, authOK := r.BasicAuth()
		if !authOK && !authorized {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		if !authorized && s.authUser != "" {
			userMatch := subtle.ConstantTimeCompare([]byte(username), []byte(s.authUser)) == 1
			passMatch := subtle.ConstantTimeCompare([]byte(password), []byte(s.authPass)) == 1
			if userMatch && passMatch {
				authorized = true
			}
		}

		if !authorized && s.htpasswdFile != nil {
			authorized = s.htpasswdFile.Match(username, password)
		}

		if !authorized {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		h.ServeHTTP(w, r)
	}
}
