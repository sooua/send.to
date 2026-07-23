package server

import "strings"

// Share tokens and deletion tokens are bearer secrets: anyone holding one can
// download or delete the upload. Writing them verbatim into the access log
// hands every file to whoever can read the logs — which, once logs are shipped
// to an aggregator, is a much wider audience than the operator. Everything
// that reaches a log goes through these helpers instead.

// downloadActions are the optional first path segment of a download URL.
var downloadActions = map[string]bool{
	"download": true,
	"get":      true,
	"inline":   true,
}

// maskToken keeps enough of a token to correlate log lines with each other
// without leaving enough to fetch the file.
func maskToken(token string) string {
	const keep = 2

	if len(token) <= keep {
		return strings.Repeat("*", len(token))
	}

	return token[:keep] + strings.Repeat("*", len(token)-keep)
}

// redactPath masks the secret segments of a request path, leaving the shape of
// the request (and the filename, which is not a secret) intact.
func redactPath(p string) string {
	// Archive requests embed a comma-separated list of token/filename pairs.
	if strings.HasPrefix(p, "/(") {
		if i := strings.LastIndex(p, ")"); i > 0 {
			return "/(...)" + p[i+1:]
		}
		return "/(...)"
	}

	trimmed := strings.TrimPrefix(p, "/")
	if trimmed == "" {
		return p
	}

	segments := strings.Split(trimmed, "/")

	tokenIdx := 0
	if downloadActions[segments[0]] {
		tokenIdx = 1
	}

	// A token is only a token when a filename follows it; a single segment is
	// an upload path (PUT /{filename}) or a static asset.
	if len(segments) <= tokenIdx+1 {
		return p
	}

	segments[tokenIdx] = maskToken(segments[tokenIdx])

	// DELETE /{token}/{filename}/{deletionToken}
	if len(segments) > tokenIdx+2 {
		segments[tokenIdx+2] = maskToken(segments[tokenIdx+2])
	}

	return "/" + strings.Join(segments, "/")
}
