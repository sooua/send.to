package client

// The owner token: an upload history that survives the machine that made it.
//
// history.json only knows what this machine uploaded. A CI runner that has been
// torn down, or a laptop that was reinstalled, takes every delete link with it.
//
// So the client keeps one master secret and derives a per-server token from it,
// which it sends with each upload. The server stores only the hash, and hands
// the list back to whoever presents the token again — from any machine. Still
// no account, no password, nothing to reset.
//
// The token is derived per server rather than shared, so an instance you upload
// to learns nothing it could replay against a different one.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemoteEntry is one upload as the server remembers it.
type RemoteEntry struct {
	URL          string     `json:"url"`
	DeleteURL    string     `json:"delete_url"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	ContentType  string     `json:"content_type,omitempty"`
	Encrypted    bool       `json:"encrypted"`
	MaxDownloads *int       `json:"max_downloads,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	UploadedAt   time.Time  `json:"uploaded_at"`
}

// ownerKeyPath is where the master secret lives.
func ownerKeyPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "owner.key"), nil
}

// masterOwnerKey reads the master secret, creating one on first use.
func masterOwnerKey() ([]byte, error) {
	path, err := ownerKeyPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's own config dir
	if err == nil {
		key, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(data)))
		if decodeErr == nil && len(key) == 32 {
			return key, nil
		}
		return nil, errors.New("owner.key is corrupt; delete it to start a new upload identity")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	// The secret is a capability over every upload made with it.
	if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(key)+"\n"), 0600); err != nil {
		return nil, err
	}

	return key, nil
}

// OwnerToken returns the token to present to one server. SENDTO_OWNER_TOKEN
// overrides it, which is how several CI machines share one upload list.
func OwnerToken(serverURL string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("SENDTO_OWNER_TOKEN")); v != "" {
		return v, nil
	}

	key, err := masterOwnerKey()
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, key)
	_, _ = io.WriteString(mac, strings.TrimSuffix(serverURL, "/"))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// withOwner attaches the token. It is deliberately not part of the common
// request setup: a download can be aimed at somebody else's server, and the
// token has no business travelling there.
func (c *Client) withOwner(req *http.Request) {
	if c.OwnerToken != "" {
		req.Header.Set("X-Owner-Token", c.OwnerToken)
	}
}

// RemoteList returns what this owner has uploaded to the server, newest first.
func (c *Client) RemoteList(ctx context.Context) ([]RemoteEntry, error) {
	if c.OwnerToken == "" {
		return nil, errors.New("no owner token configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/owner/files", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	c.withOwner(req)

	res, err := c.do(req)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusMethodNotAllowed) {
			return nil, errors.New("this server does not keep an upload list")
		}
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	var body struct {
		Files []RemoteEntry `json:"files"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}

	return body.Files, nil
}
