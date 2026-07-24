package server

// Downloads are the only thing an instance serves at scale, and every one of
// them answered `Cache-Control: no-store`. That is the correct default for a
// service whose links are secret and whose files can be revoked, but it also
// means a CDN in front of the instance is decoration: every byte of every
// repeat download comes off the origin.
//
// An upload is immutable once stored, so the cheap half of this is
// unconditional — an ETag, and a 304 for anyone who asks with it. That costs an
// operator nothing and skips the storage read as well as the body.
//
// Storing the response is the half that needs an opinion, so it is opt-in via
// --cache-max-age, and even then only for uploads where caching cannot change
// behaviour:
//
//   - Max-Downloads counts *completed* downloads. A cached copy is served
//     without the origin ever knowing, so a download limit and a shared cache
//     cannot both be honoured. Limited uploads stay no-store.
//   - Server-side encrypted uploads are decrypted per request against a
//     password header. Vary would cover it, but caches handle Vary on a
//     non-standard header poorly, and getting this wrong means serving one
//     visitor's plaintext to another. They stay no-store.
//
// The cost an operator accepts for the rest: a deleted file stays reachable
// through the cache for up to --cache-max-age. That is why the default is off.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// uploadETag identifies the bytes a download of this upload produces.
//
// Nothing ever rewrites an upload in place, so the identity of the upload is
// the identity of its content: same token, same filename, same declared length.
// It is hashed rather than assembled so the header cannot be read back as a
// deletion token, a size, or anything else about the file.
func uploadETag(token, filename string, m metadata, decrypting bool) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		token,
		filename,
		strconv.FormatInt(m.ContentLength, 10),
		strconv.FormatBool(decrypting),
	}, "\x00")))

	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// etagMatches reports whether an If-None-Match header covers etag.
//
// Weak comparison, which is what RFC 9110 requires for If-None-Match: a weak
// validator still identifies the same bytes here, because there is no other way
// for two responses to share a token, a filename and a length.
func etagMatches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}

	want := strings.TrimPrefix(etag, "W/")

	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if strings.TrimPrefix(candidate, "W/") == want {
			return true
		}
	}

	return false
}

// cacheControl returns the Cache-Control value for a download of m.
func (s *Server) cacheControl(m metadata) string {
	if s.cacheMaxAge <= 0 || m.MaxDownloads != -1 || m.Encrypted {
		return "no-store"
	}

	age := s.cacheMaxAge

	// A cache must not outlive the link. Max-Days is the promise the uploader
	// was given, and a cached copy that survives it breaks that promise in the
	// one direction that matters.
	if !m.MaxDate.IsZero() {
		remaining := time.Until(m.MaxDate)
		if remaining <= 0 {
			return "no-store"
		}
		if remaining < age {
			age = remaining
		}
	}

	return fmt.Sprintf("public, max-age=%d, immutable", int(age.Seconds()))
}

// serveNotModified attaches the validator and answers 304 if the client
// already holds this upload, reporting whether it did.
//
// It runs before the storage read, which is the point: a revalidating client
// costs the backend nothing, not even a Head.
//
// Only the 304 path sets Cache-Control. Callers set it themselves once the
// object is known to exist, so that a 404 or a backend failure can never
// inherit a freshness lifetime and get cached in place of the file.
func (s *Server) serveNotModified(w http.ResponseWriter, r *http.Request, token, filename string, m metadata, decrypting bool) bool {
	etag := uploadETag(token, filename, m, decrypting)
	w.Header().Set("ETag", etag)

	if !etagMatches(r.Header.Get("If-None-Match"), etag) {
		return false
	}

	w.Header().Set("Cache-Control", s.cacheControl(m))
	w.WriteHeader(http.StatusNotModified)

	return true
}
