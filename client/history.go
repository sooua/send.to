package client

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// historyLimit caps the file so a busy CI account cannot grow it without
// bound. Oldest entries fall off first.
const historyLimit = 500

// Entry is one upload this machine made. transfer.sh keeps no such record —
// lose the link and the file is unreachable and undeletable forever.
type Entry struct {
	URL          string     `json:"url"`
	DeleteURL    string     `json:"delete_url,omitempty"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	Encrypted    bool       `json:"encrypted"`
	MaxDownloads *int       `json:"max_downloads,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	UploadedAt   time.Time  `json:"uploaded_at"`
	Server       string     `json:"server"`
}

// Expired reports whether the entry is past its recorded expiry. Download
// limits are not tracked locally — only the server knows the running count.
func (e Entry) Expired() bool {
	return e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt)
}

// History is the local record of uploads, newest first.
type History struct {
	Entries []Entry `json:"entries"`
}

func historyPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

// LoadHistory reads the upload history, returning an empty one when there is
// no file yet. A corrupt file is not fatal: losing history must never stop an
// upload from happening.
func LoadHistory() (*History, error) {
	path, err := historyPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's own config dir
	if errors.Is(err, os.ErrNotExist) {
		return &History{}, nil
	}
	if err != nil {
		return nil, err
	}

	var h History
	if err := json.Unmarshal(data, &h); err != nil {
		return &History{}, nil
	}

	return &h, nil
}

// Save writes the history, newest first, truncated to historyLimit.
func (h *History) Save() error {
	path, err := historyPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	sort.SliceStable(h.Entries, func(i, j int) bool {
		return h.Entries[i].UploadedAt.After(h.Entries[j].UploadedAt)
	})

	if len(h.Entries) > historyLimit {
		h.Entries = h.Entries[:historyLimit]
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}

	// Delete URLs are capabilities, so keep the file to the owner.
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// Add records an upload.
func (h *History) Add(server string, r *Result) {
	h.Entries = append([]Entry{{
		URL:          r.URL,
		DeleteURL:    r.DeleteURL,
		Filename:     r.Filename,
		Size:         r.Size,
		Encrypted:    r.Encrypted,
		MaxDownloads: r.MaxDownloads,
		ExpiresAt:    r.ExpiresAt,
		UploadedAt:   time.Now(),
		Server:       server,
	}}, h.Entries...)
}

// Remove drops the entry for a URL and reports whether one was found.
func (h *History) Remove(url string) bool {
	for i, e := range h.Entries {
		if e.URL == url {
			h.Entries = append(h.Entries[:i], h.Entries[i+1:]...)
			return true
		}
	}
	return false
}

// Find returns the entry for a URL, or nil.
func (h *History) Find(url string) *Entry {
	for i := range h.Entries {
		if h.Entries[i].URL == url {
			return &h.Entries[i]
		}
	}
	return nil
}

// Prune drops entries whose recorded expiry has passed and reports how many
// were removed.
func (h *History) Prune() int {
	kept := h.Entries[:0]
	removed := 0

	for _, e := range h.Entries {
		if e.Expired() {
			removed++
			continue
		}
		kept = append(kept, e)
	}

	h.Entries = kept
	return removed
}
