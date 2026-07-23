package client

// Resumable uploads.
//
// A plain PUT is one request: lose the connection at 90% of a 5 GB artefact and
// the 4.5 GB already transferred are gone. This splits the body into chunks that
// the server accepts all-or-nothing, so a failure costs at most one chunk and a
// retry continues from the offset the server reports rather than from zero.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

const (
	// UploadChunkSize is how much one PATCH carries. Large enough that the
	// per-request overhead disappears on a fast link, small enough that a
	// failure on a slow one does not throw away minutes of transfer.
	UploadChunkSize = 8 << 20

	// ResumableThreshold is where chunking starts paying for its extra round
	// trips. Below it, a failed upload is cheap to simply repeat.
	ResumableThreshold = 16 << 20

	// uploadChunkAttempts bounds retries of a single chunk before the upload
	// gives up. The session survives on the server either way, so the user can
	// run the same command again later and continue.
	uploadChunkAttempts = 5
)

// ErrResumableUnsupported means the server has no resumable upload endpoint —
// an older send.to, or transfer.sh. The caller should fall back to a plain PUT.
var ErrResumableUnsupported = errors.New("server does not support resumable uploads")

// Session is an upload in progress on the server.
type Session struct {
	// URL is the session endpoint; it is the only thing needed to resume.
	URL string
	// Offset is how many bytes the server already holds.
	Offset int64
	// Length is the total the session was created for.
	Length int64
}

// OffsetConflictError is the server saying it is at a different offset than the
// chunk assumed — after a retry, usually, when the first attempt did land.
type OffsetConflictError struct {
	Offset int64
}

func (e *OffsetConflictError) Error() string {
	return fmt.Sprintf("server is at byte %d", e.Offset)
}

// ChunkSource produces the upload body from a byte offset. Closing the returned
// reader must release whatever produced it, so a chunk that is abandoned
// halfway does not leak a goroutine.
type ChunkSource func(offset int64) (io.ReadCloser, error)

// CreateSession opens a resumable upload of size bytes.
func (c *Client) CreateSession(ctx context.Context, name string, size int64, opts UploadOptions) (*Session, error) {
	if opts.Password != "" {
		return nil, errors.New("server-side encryption cannot be resumed; use --e2e, or a smaller file")
	}

	target := c.BaseURL + "/upload/" + url.PathEscape(path.Base(name))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Upload-Length", strconv.FormatInt(size, 10))
	c.withOwner(req)

	if opts.Days > 0 {
		req.Header.Set("Max-Days", strconv.Itoa(opts.Days))
	}
	if opts.MaxDownloads > 0 {
		req.Header.Set("Max-Downloads", strconv.Itoa(opts.MaxDownloads))
	}

	res, err := c.do(req)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusMethodNotAllowed) {
			return nil, ErrResumableUnsupported
		}
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	var body struct {
		UploadURL string `json:"upload_url"`
		Length    int64  `json:"length"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)

	sessionURL := body.UploadURL
	if sessionURL == "" {
		sessionURL = res.Header.Get("Location")
	}
	if sessionURL == "" {
		return nil, errors.New("server did not return an upload session URL")
	}

	return &Session{URL: sessionURL, Length: size}, nil
}

// SessionStatus asks how much of an upload the server holds. A session that has
// expired or was never created reports ErrResumableUnsupported's sibling — a
// plain 404 — which the caller treats as "start again".
func (c *Client) SessionStatus(ctx context.Context, sessionURL string) (*Session, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, sessionURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	offset, err := strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		return nil, errors.New("server did not report an upload offset")
	}

	length, _ := strconv.ParseInt(res.Header.Get("Upload-Length"), 10, 64)

	return &Session{URL: sessionURL, Offset: offset, Length: length}, nil
}

// sendChunk uploads one span. It returns the upload's Result once the final
// chunk completes the file, and the server's new offset otherwise.
func (c *Client) sendChunk(ctx context.Context, sess *Session, body io.Reader, start, n int64) (*Result, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, sess.URL, io.NopCloser(body))
	if err != nil {
		return nil, 0, err
	}

	req.ContentLength = n
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+n-1, sess.Length))

	c.auth(req)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode == http.StatusConflict {
		offset, parseErr := strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64)
		if parseErr != nil {
			return nil, 0, errors.New("server rejected the chunk without reporting its offset")
		}
		return nil, 0, &OffsetConflictError{Offset: offset}
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, 0, &APIError{StatusCode: res.StatusCode, Body: string(raw)}
	}

	if res.StatusCode == http.StatusNoContent {
		offset, parseErr := strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64)
		if parseErr != nil {
			return nil, 0, errors.New("server did not report an upload offset")
		}
		return nil, offset, nil
	}

	var result Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, 0, fmt.Errorf("could not parse server response: %w", err)
	}
	if result.URL == "" {
		return nil, 0, errors.New("server did not return an upload URL")
	}
	if result.DeleteURL == "" {
		result.DeleteURL = res.Header.Get("X-Url-Delete")
	}

	return &result, sess.Length, nil
}

// UploadSession drives a session to completion, retrying a failed chunk from
// whatever offset the server turns out to be at.
func (c *Client) UploadSession(ctx context.Context, sess *Session, src ChunkSource, progress func(sent int64)) (*Result, error) {
	attempts := 0

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if sess.Offset >= sess.Length {
			// Every byte is on the server but no final response was seen: the
			// completing chunk's reply was lost. Nothing left to send.
			return nil, errors.New("upload is complete on the server but the confirmation was lost; run `send ls` on the server or upload again")
		}

		n := int64(UploadChunkSize)
		if remaining := sess.Length - sess.Offset; remaining < n {
			n = remaining
		}

		start := sess.Offset

		result, offset, err := c.sendOneChunk(ctx, sess, src, start, n)
		if err == nil {
			sess.Offset = offset
			if progress != nil {
				progress(sess.Offset)
			}
			if result != nil {
				return result, nil
			}
			attempts = 0
			continue
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		var conflict *OffsetConflictError
		if errors.As(err, &conflict) {
			sess.Offset = conflict.Offset
			if progress != nil {
				progress(sess.Offset)
			}
		} else if !retryableUploadError(err) {
			return nil, err
		}

		attempts++
		if attempts >= uploadChunkAttempts {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempts) * time.Second):
		}

		// Resynchronise: after a broken connection only the server knows how
		// much of the chunk it kept.
		if status, statusErr := c.SessionStatus(ctx, sess.URL); statusErr == nil {
			sess.Offset = status.Offset
			if progress != nil {
				progress(sess.Offset)
			}
		}
	}
}

func (c *Client) sendOneChunk(ctx context.Context, sess *Session, src ChunkSource, start, n int64) (*Result, int64, error) {
	body, err := src(start)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = body.Close() }()

	return c.sendChunk(ctx, sess, io.LimitReader(body, n), start, n)
}

// retryableUploadError reports whether trying the same chunk again could work.
// Transport failures and server-side faults can; a rejected request cannot.
func retryableUploadError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		// A transport error: connection reset, timeout, DNS blip.
		return true
	}

	// 408 is what the server answers when a chunk body arrived short, which is
	// precisely the case worth retrying.
	return apiErr.StatusCode == http.StatusRequestTimeout || apiErr.StatusCode >= 500
}
