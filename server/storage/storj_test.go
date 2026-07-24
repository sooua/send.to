package storage

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"storj.io/uplink"
)

// Storj has no offline fake worth the name: uplink talks to a satellite over
// its own protocol, and the pieces that break — expiry handling, the listing
// Purge sweeps — are exactly the pieces a stub would have to invent. So the
// round-trip tests run against a real project when one is configured:
//
//	STORJ_TEST_ACCESS=<access grant> go test ./server/storage/...
//
// Without it they skip, and CI stays offline. The tests below the fold need no
// project and always run.
func storjTestStorage(t *testing.T) *StorjStorage {
	t.Helper()

	access := os.Getenv("STORJ_TEST_ACCESS")
	if access == "" {
		t.Skip("set STORJ_TEST_ACCESS to run the Storj backend tests")
	}

	store, err := NewStorjStorage(t.Context(), access, envOr("STORJ_TEST_BUCKET", "sendto-test"), 0,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func TestStorjRoundTrip(t *testing.T) {
	store := storjTestStorage(t)
	ctx := t.Context()

	token := "rt" + randomSuffix()
	payload := bytes.Repeat([]byte("j"), 4096)

	if err := store.Put(ctx, token, "a.bin", bytes.NewReader(payload), "application/octet-stream", uint64(len(payload))); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Delete(ctx, token, "a.bin") }()

	length, err := store.Head(ctx, token, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if length != uint64(len(payload)) {
		t.Errorf("Head = %d, want %d", length, len(payload))
	}

	reader, _, err := store.Get(ctx, token, "a.bin", ParseRange("bytes=4000-"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()

	if len(got) != 96 {
		t.Errorf("ranged Get returned %d bytes, want 96", len(got))
	}
}

func TestStorjPurgeRemovesObjectsWithoutExpiry(t *testing.T) {
	store := storjTestStorage(t)
	ctx := t.Context()

	token := "purge" + randomSuffix()

	// purgeDays is zero on the test storage, so this object carries no expiry —
	// which is the case the sweep exists for. Storj drops objects it was told to
	// expire; it cannot drop the ones uploaded before --purge-days was set.
	if err := store.Put(ctx, token, "a.bin", bytes.NewReader([]byte("old")), "text/plain", 3); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Delete(ctx, token, "a.bin") }()

	// A negative age makes the cutoff the future, so everything is expired.
	if err := store.Purge(ctx, -time.Minute); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Head(ctx, token, "a.bin"); !store.IsNotExist(err) {
		t.Fatalf("object survived a purge that covered it: err = %v", err)
	}
}

func TestStorjPurgeKeepsFreshObjects(t *testing.T) {
	store := storjTestStorage(t)
	ctx := t.Context()

	token := "fresh" + randomSuffix()

	if err := store.Put(ctx, token, "a.bin", bytes.NewReader([]byte("new")), "text/plain", 3); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Delete(ctx, token, "a.bin") }()

	if err := store.Purge(ctx, 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Head(ctx, token, "a.bin"); err != nil {
		t.Fatalf("purge deleted an object that had not expired: %v", err)
	}
}

func TestStorjUsageIsUnsupported(t *testing.T) {
	var store StorjStorage

	if _, err := store.Usage(t.Context()); err != ErrUsageUnsupported {
		t.Fatalf("Usage error = %v, want ErrUsageUnsupported — the storage quota must refuse to start, not silently allow everything", err)
	}
}

func TestStorjIsNotExist(t *testing.T) {
	var store StorjStorage

	if store.IsNotExist(nil) {
		t.Error("IsNotExist(nil) is true, so a successful call would be read as a 404")
	}
	if !store.IsNotExist(uplink.ErrObjectNotFound) {
		t.Error("a missing object is not recognised, so downloads answer 500 instead of 404")
	}
	if store.IsNotExist(io.ErrUnexpectedEOF) {
		t.Error("an unrelated error is treated as a missing object, hiding real failures behind a 404")
	}
}
