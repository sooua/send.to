package server

// Server-side upload history, without an account.
//
// The client's local history solves "what did I upload from this laptop". It
// cannot answer "what did I upload from the build box last week", because the
// record lives on the machine that made it — and a CI runner that has been torn
// down takes every delete link with it.
//
// An owner token fixes that without introducing accounts. The client generates
// a random secret once, keeps it, and sends it with every upload:
//
//	X-Owner-Token: <secret>
//
// The server stores sha256(secret) and nothing else. The hash names a small
// index object listing that owner's uploads, so `send ls --remote` and
// `send rm` work from any machine holding the secret — and from none that does
// not. There is no password to reset because there is no account: lose the
// secret and the uploads simply become anonymous again, exactly as they are
// today.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sooua/send.to/server/storage"
)

const (
	// ownerIndexToken is the storage token the per-owner index objects live
	// under. It is not a valid share token — every download path requires a
	// `<filename>.metadata` sidecar, which an index has not got — so the index
	// cannot be fetched through the normal download routes.
	ownerIndexToken = ".owners"

	// ownerIndexLimit bounds an index. A busy CI account would otherwise grow
	// one object without limit; oldest entries fall off first.
	ownerIndexLimit = 200

	// ownerTokenMinLength keeps a hand-written token from being guessable. The
	// client generates 43 characters of base64.
	ownerTokenMinLength = 22
)

// ownerEntry is one upload as the owner index remembers it.
type ownerEntry struct {
	URL          string     `json:"url"`
	DeleteURL    string     `json:"delete_url"`
	Token        string     `json:"token"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	ContentType  string     `json:"content_type,omitempty"`
	Encrypted    bool       `json:"encrypted"`
	MaxDownloads *int       `json:"max_downloads,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	UploadedAt   time.Time  `json:"uploaded_at"`
}

// ownerHashFromHeaders turns the caller's secret into the index name. The
// secret itself is never stored, logged or echoed back.
func ownerHashFromHeaders(h http.Header) string {
	secret := strings.TrimSpace(h.Get("X-Owner-Token"))
	if len(secret) < ownerTokenMinLength {
		return ""
	}

	sum := sha256.Sum256([]byte(secret))

	return hex.EncodeToString(sum[:])
}

func ownerIndexName(hash string) string {
	return hash + ".json"
}

// readOwnerIndex loads an owner's uploads. A missing index is an empty one:
// asking about an owner who has uploaded nothing is not an error.
func (s *Server) readOwnerIndex(ctx context.Context, hash string) ([]ownerEntry, error) {
	r, _, err := s.storage.Get(ctx, ownerIndexToken, ownerIndexName(hash), nil)
	defer storage.CloseCheck(r)

	if s.storage.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var entries []ownerEntry
	if err := json.NewDecoder(r).Decode(&entries); err != nil {
		return nil, err
	}

	return entries, nil
}

func (s *Server) writeOwnerIndex(ctx context.Context, hash string, entries []ownerEntry) error {
	if len(entries) > ownerIndexLimit {
		entries = entries[:ownerIndexLimit]
	}

	buffer := &bytes.Buffer{}
	if err := json.NewEncoder(buffer).Encode(entries); err != nil {
		return err
	}

	return s.storage.Put(ctx, ownerIndexToken, ownerIndexName(hash), buffer, "application/json", uint64(buffer.Len()))
}

// recordOwnership adds one upload to its owner's index. Best effort: an index
// that cannot be written must not fail the upload it describes, because the
// share URL is already valid by then.
func (s *Server) recordOwnership(r *http.Request, result uploadResult, m metadata, uploadToken string) {
	if m.OwnerHash == "" {
		return
	}

	ctx := context.WithoutCancel(r.Context())

	s.lock(ownerIndexToken, m.OwnerHash)
	defer s.unlock(ownerIndexToken, m.OwnerHash)

	entries, err := s.readOwnerIndex(ctx, m.OwnerHash)
	if err != nil {
		s.logger.Error("Could not read the owner index", "error", err)
		return
	}

	entry := ownerEntry{
		URL:          result.URL,
		DeleteURL:    result.DeleteURL,
		Token:        uploadToken,
		Filename:     result.Filename,
		Size:         result.Size,
		ContentType:  result.ContentType,
		Encrypted:    result.Encrypted,
		MaxDownloads: result.MaxDownloads,
		ExpiresAt:    result.ExpiresAt,
		UploadedAt:   time.Now().UTC(),
	}

	if err := s.writeOwnerIndex(ctx, m.OwnerHash, append([]ownerEntry{entry}, entries...)); err != nil {
		s.logger.Error("Could not update the owner index", "error", err)
	}
}

// forgetOwnership drops an upload from its owner's index once the upload is
// gone, so the list never offers a link that cannot work.
func (s *Server) forgetOwnership(ctx context.Context, hash, uploadToken, filename string) {
	if hash == "" {
		return
	}

	s.lock(ownerIndexToken, hash)
	defer s.unlock(ownerIndexToken, hash)

	entries, err := s.readOwnerIndex(ctx, hash)
	if err != nil || len(entries) == 0 {
		return
	}

	kept := make([]ownerEntry, 0, len(entries))
	for _, e := range entries {
		if e.Token == uploadToken && e.Filename == filename {
			continue
		}
		kept = append(kept, e)
	}

	if len(kept) == len(entries) {
		return
	}

	if err := s.writeOwnerIndex(ctx, hash, kept); err != nil {
		s.logger.Error("Could not update the owner index", "error", err)
	}
}

// ownerFilesHandler lists what the holder of a token has uploaded.
//
// Entries whose upload is gone are dropped as they are found, so the index
// heals itself rather than accumulating links to files that expired, ran out of
// downloads, or were purged by age.
func (s *Server) ownerFilesHandler(w http.ResponseWriter, r *http.Request) {
	hash := ownerHashFromHeaders(r.Header)
	if hash == "" {
		s.httpError(w, r, http.StatusBadRequest, msgOwnerTokenShort, ownerTokenMinLength)
		return
	}

	s.lock(ownerIndexToken, hash)
	defer s.unlock(ownerIndexToken, hash)

	entries, err := s.readOwnerIndex(r.Context(), hash)
	if err != nil {
		s.logger.Error("Could not read the owner index", "error", err)
		s.httpError(w, r, http.StatusInternalServerError, msgOwnerListFailed)
		return
	}

	live := make([]ownerEntry, 0, len(entries))
	for _, e := range entries {
		if _, err := s.storage.Head(r.Context(), e.Token, fmt.Sprintf("%s.metadata", e.Filename)); err != nil {
			continue
		}
		live = append(live, e)
	}

	if len(live) != len(entries) {
		if err := s.writeOwnerIndex(r.Context(), hash, live); err != nil {
			s.logger.Error("Could not prune the owner index", "error", err)
		}
	}

	w.Header().Set("Cache-Control", "no-store")

	if wantsJSON(r.Header) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Files []ownerEntry `json:"files"`
		}{Files: live})
		return
	}

	// Plain text is one URL per line, which is what a shell loop wants.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, e := range live {
		_, _ = fmt.Fprintln(w, e.URL)
	}
}
