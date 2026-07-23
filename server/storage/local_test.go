package storage

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"
)

func newLocal(t *testing.T) *LocalStorage {
	t.Helper()

	store, err := NewLocalStorage(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func TestLocalStorageRoundTrip(t *testing.T) {
	store := newLocal(t)
	ctx := t.Context()

	payload := []byte("hello from local storage")

	if err := store.Put(ctx, "tok", "a.txt", bytes.NewReader(payload), "text/plain", uint64(len(payload))); err != nil {
		t.Fatal(err)
	}

	length, err := store.Head(ctx, "tok", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if length != uint64(len(payload)) {
		t.Errorf("Head = %d, want %d", length, len(payload))
	}

	reader, length, err := store.Get(ctx, "tok", "a.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()

	if !bytes.Equal(got, payload) {
		t.Errorf("Get returned %q", got)
	}
	if length != uint64(len(payload)) {
		t.Errorf("Get length = %d, want %d", length, len(payload))
	}

	// A range starts where it was asked to and reports the remaining length.
	rng := ParseRange("bytes=6-")
	reader, length, err = store.Get(ctx, "tok", "a.txt", rng)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(reader)
	_ = reader.Close()

	if string(got) != string(payload[6:]) {
		t.Errorf("ranged Get returned %q", got)
	}
	if length != uint64(len(payload)-6) {
		t.Errorf("ranged length = %d, want %d", length, len(payload)-6)
	}

	if err := store.Delete(ctx, "tok", "a.txt"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Head(ctx, "tok", "a.txt"); !store.IsNotExist(err) {
		t.Errorf("after Delete, Head returned %v", err)
	}
}

// Usage is what the total-size quota is seeded from, so an undercount is a
// quota that never bites.
func TestLocalStorageUsage(t *testing.T) {
	store := newLocal(t)
	ctx := t.Context()

	used, err := store.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if used != 0 {
		t.Errorf("empty storage reports %d bytes", used)
	}

	first := bytes.Repeat([]byte("x"), 500)
	second := bytes.Repeat([]byte("y"), 250)

	if err := store.Put(ctx, "one", "a.bin", bytes.NewReader(first), "application/octet-stream", uint64(len(first))); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "two", "b.bin", bytes.NewReader(second), "application/octet-stream", uint64(len(second))); err != nil {
		t.Fatal(err)
	}

	used, err = store.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if used != 750 {
		t.Errorf("Usage = %d, want 750 — files in different token directories must all count", used)
	}

	if err := store.Delete(ctx, "one", "a.bin"); err != nil {
		t.Fatal(err)
	}

	used, err = store.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if used != 250 {
		t.Errorf("Usage after a delete = %d, want 250", used)
	}
}

// A base directory that does not exist yet is an empty one, not an error: the
// quota is seeded before the first upload has created anything.
func TestLocalStorageUsageOnMissingDirectory(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir()+"/not-created-yet", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	used, err := store.Usage(t.Context())
	if err != nil {
		t.Fatalf("Usage on a missing directory: %v", err)
	}
	if used != 0 {
		t.Errorf("Usage = %d, want 0", used)
	}
}

func TestLocalStoragePurgeRemovesEmptyTokenDirs(t *testing.T) {
	store := newLocal(t)
	ctx := t.Context()

	payload := []byte("old")
	if err := store.Put(ctx, "stale", "old.txt", bytes.NewReader(payload), "text/plain", uint64(len(payload))); err != nil {
		t.Fatal(err)
	}

	// Everything older than zero duration is everything.
	if err := store.Purge(ctx, 0); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(store.basedir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("purge left %d entries behind", len(entries))
	}
}
