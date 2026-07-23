// Package client is the send.to API client used by the `send` command.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// Version is the client build version, set from main at startup.
var Version = "dev"

// Client talks to one send.to instance.
type Client struct {
	BaseURL  string
	Username string
	Password string
	// OwnerToken identifies this uploader to the server without an account, so
	// the uploads it makes can be listed and deleted from another machine.
	OwnerToken string
	HTTP       *http.Client
}

// New returns a client for baseURL. A zero timeout is deliberate: uploads and
// downloads are unbounded in size, so the per-request deadline has to come
// from the caller's context instead.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTP:    &http.Client{},
	}
}

// Result describes a stored upload. It mirrors the server's JSON response.
type Result struct {
	URL          string     `json:"url"`
	DeleteURL    string     `json:"delete_url"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	ContentType  string     `json:"content_type,omitempty"`
	Encrypted    bool       `json:"encrypted"`
	MaxDownloads *int       `json:"max_downloads,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// UploadOptions are the per-upload limits the server understands.
type UploadOptions struct {
	Days         int
	MaxDownloads int
	Password     string
}

// Info is what a HEAD request reveals about a stored file. Fetching it does
// not spend a download from the file's Max-Downloads budget.
type Info struct {
	Filename           string
	ContentType        string
	Size               int64
	RemainingDownloads string
	RemainingDays      string
	SupportsRange      bool
}

// APIError carries a non-2xx response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("server returned %s", http.StatusText(e.StatusCode))
	}
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	return fmt.Sprintf("server returned %d: %s", e.StatusCode, body)
}

// NotFound reports whether err is a 404, which for this API means the upload
// expired, ran out of downloads, or never existed.
func NotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func (c *Client) auth(req *http.Request) {
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	req.Header.Set("User-Agent", "send/"+Version)
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	c.auth(req)

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		_ = res.Body.Close()
		return nil, &APIError{StatusCode: res.StatusCode, Body: string(body)}
	}

	return res, nil
}

// Upload stores one file. size may be -1 when unknown, in which case the body
// is sent chunked and the server spools it to disk to learn the length.
func (c *Client) Upload(ctx context.Context, name string, body io.Reader, size int64, opts UploadOptions) (*Result, error) {
	target := c.BaseURL + "/" + url.PathEscape(path.Base(name))

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, body)
	if err != nil {
		return nil, err
	}

	req.ContentLength = size
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/octet-stream")
	c.withOwner(req)

	if opts.Days > 0 {
		req.Header.Set("Max-Days", strconv.Itoa(opts.Days))
	}
	if opts.MaxDownloads > 0 {
		req.Header.Set("Max-Downloads", strconv.Itoa(opts.MaxDownloads))
	}
	if opts.Password != "" {
		req.Header.Set("X-Encrypt-Password", opts.Password)
	}

	res, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	var result Result
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("could not parse server response: %w", err)
	}

	// A server too old to speak JSON answers with a bare URL, which decodes
	// into an empty struct rather than failing.
	if result.URL == "" {
		return nil, errors.New("server did not return an upload URL (is it running a current version?)")
	}

	if result.DeleteURL == "" {
		result.DeleteURL = res.Header.Get("X-Url-Delete")
	}

	return &result, nil
}

// Stat fetches a file's metadata without spending a download.
func (c *Client) Stat(ctx context.Context, fileURL string) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	size, _ := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)

	return &Info{
		Filename:           filenameFromURL(fileURL),
		ContentType:        res.Header.Get("Content-Type"),
		Size:               size,
		RemainingDownloads: headerOr(res, "X-Remaining-Downloads", "n/a"),
		RemainingDays:      headerOr(res, "X-Remaining-Days", "n/a"),
		SupportsRange:      res.Header.Get("Accept-Ranges") == "bytes",
	}, nil
}

// DownloadOptions controls a single Download call.
type DownloadOptions struct {
	// Password decrypts a server-side encrypted upload.
	Password string
	// Offset resumes the transfer, skipping bytes already on disk. The server
	// does not charge a resumed range against Max-Downloads.
	Offset int64
	// Progress, when set, is called with the running byte count.
	Progress func(written, total int64)
}

// Download streams a file into w. It returns the number of bytes written and
// whether the server honoured the requested resume offset — a server that
// ignores it restarts from zero, and the caller must discard what it had.
func (c *Client) Download(ctx context.Context, fileURL string, w io.Writer, opts DownloadOptions) (written int64, resumed bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return 0, false, err
	}

	if opts.Password != "" {
		req.Header.Set("X-Decrypt-Password", opts.Password)
	}
	if opts.Offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(opts.Offset, 10)+"-")
	}

	res, err := c.do(req)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = res.Body.Close() }()

	resumed = res.StatusCode == http.StatusPartialContent

	total := res.ContentLength
	if resumed && total > 0 {
		total += opts.Offset
	}

	dst := w
	if opts.Progress != nil {
		base := int64(0)
		if resumed {
			base = opts.Offset
		}
		dst = &progressWriter{w: w, total: total, written: base, report: opts.Progress}
	}

	written, err = io.Copy(dst, res.Body)
	return written, resumed, err
}

// Delete removes an upload using the deletion URL returned when it was stored.
func (c *Client) Delete(ctx context.Context, deleteURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}

	res, err := c.do(req)
	if err != nil {
		// The upload already being gone is the desired end state.
		if NotFound(err) {
			return nil
		}
		return err
	}

	return res.Body.Close()
}

type progressWriter struct {
	w       io.Writer
	total   int64
	written int64
	report  func(written, total int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.written += int64(n)
	p.report(p.written, p.total)
	return n, err
}

func headerOr(res *http.Response, name, fallback string) string {
	if v := res.Header.Get(name); v != "" {
		return v
	}
	return fallback
}

func filenameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return path.Base(raw)
	}
	name, err := url.PathUnescape(path.Base(u.Path))
	if err != nil {
		return path.Base(u.Path)
	}
	return name
}

// FileSize returns the size of a file, or -1 when it is not a regular file
// (a pipe or device has no length to declare up front).
func FileSize(f *os.File) int64 {
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return -1
	}
	return info.Size()
}
