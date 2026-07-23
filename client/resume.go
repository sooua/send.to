package client

// Where a resumable upload is remembered between runs.
//
// The server keeps the bytes; this keeps the one thing needed to find them
// again — the session URL — plus, for an end-to-end encrypted upload, the key
// and nonce prefix, because a resumed transfer has to continue the exact same
// ciphertext rather than start a different one.
//
// That key is written to disk, at 0600, in the same directory where history.json
// already stores every share link including its `#k=` fragment. The alternative
// is losing the ability to resume the uploads that need it most.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// pendingTTL drops entries the server would have expired anyway.
const pendingTTL = 24 * time.Hour

// PendingUpload is one interrupted upload that can still be finished.
type PendingUpload struct {
	Key        string    `json:"key"`
	SessionURL string    `json:"session_url"`
	Server     string    `json:"server"`
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Length     int64     `json:"length"`
	E2EKey     string    `json:"e2e_key,omitempty"`
	E2EPrefix  string    `json:"e2e_prefix,omitempty"`
	StartedAt  time.Time `json:"started_at"`
}

// Pending is the on-disk set of interrupted uploads.
type Pending struct {
	Uploads []PendingUpload `json:"uploads"`
}

// PendingKey identifies a file on a server well enough that a changed file
// cannot be resumed onto the bytes of the old one.
func PendingKey(server, absPath string, size int64, modTime time.Time) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%s\x00%d\x00%d", server, absPath, size, modTime.UnixNano()))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

func pendingPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "uploads.json"), nil
}

// LoadPending reads the interrupted uploads, returning an empty set when there
// is no file or it cannot be parsed — a lost resume record costs a restart, not
// an upload.
func LoadPending() (*Pending, error) {
	path, err := pendingPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's own config dir
	if errors.Is(err, os.ErrNotExist) {
		return &Pending{}, nil
	}
	if err != nil {
		return nil, err
	}

	var p Pending
	if err := json.Unmarshal(data, &p); err != nil {
		return &Pending{}, nil
	}

	return &p, nil
}

// Save writes the set back, dropping entries the server will have expired.
func (p *Pending) Save() error {
	path, err := pendingPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	kept := p.Uploads[:0]
	for _, u := range p.Uploads {
		if time.Since(u.StartedAt) < pendingTTL {
			kept = append(kept, u)
		}
	}
	p.Uploads = kept

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	// An entry can hold an encryption key, so keep the file to its owner.
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// Find returns the entry for a key, or nil.
func (p *Pending) Find(key string) *PendingUpload {
	for i := range p.Uploads {
		if p.Uploads[i].Key == key {
			return &p.Uploads[i]
		}
	}
	return nil
}

// Put records or replaces an entry.
func (p *Pending) Put(u PendingUpload) {
	if existing := p.Find(u.Key); existing != nil {
		*existing = u
		return
	}
	p.Uploads = append(p.Uploads, u)
}

// Remove drops the entry for a key.
func (p *Pending) Remove(key string) {
	for i, u := range p.Uploads {
		if u.Key == key {
			p.Uploads = append(p.Uploads[:i], p.Uploads[i+1:]...)
			return
		}
	}
}
