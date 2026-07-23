package client

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A resumed upload re-sends bytes the server does not have yet, so the
// ciphertext produced from an offset has to be exactly the tail of the
// ciphertext produced from zero. Anything else silently corrupts the file.
func TestE2EStreamReaderFromMatchesTheWholeCiphertext(t *testing.T) {
	key, err := NewE2EKey()
	if err != nil {
		t.Fatal(err)
	}

	// Sizes around the 64 KiB chunk boundary, where an off-by-one shows up.
	for _, size := range []int{0, 1, 65535, 65536, 65537, 200000} {
		plain := make([]byte, size)
		if _, err := rand.Read(plain); err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(t.TempDir(), "payload.bin")
		if err := os.WriteFile(path, plain, 0600); err != nil {
			t.Fatal(err)
		}

		stream, err := NewE2EStream(key, E2EMetadata{Name: "payload.bin", Type: "application/octet-stream"})
		if err != nil {
			t.Fatal(err)
		}

		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}

		whole, err := stream.Reader(f)
		if err != nil {
			t.Fatal(err)
		}

		full, err := io.ReadAll(whole)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()

		declared, err := stream.Size(int64(size))
		if err != nil {
			t.Fatal(err)
		}
		if declared != int64(len(full)) {
			t.Fatalf("size %d: declared %d bytes, produced %d", size, declared, len(full))
		}

		offsets := []int64{0, 1, 13, int64(len(full)) / 2, int64(len(full)) - 1}

		for _, offset := range offsets {
			if offset < 0 {
				continue
			}

			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}

			tail, err := stream.ReaderFrom(f, offset)
			if err != nil {
				t.Fatalf("size %d offset %d: %v", size, offset, err)
			}

			got, err := io.ReadAll(tail)
			_ = tail.Close()
			_ = f.Close()

			if err != nil {
				t.Fatalf("size %d offset %d: %v", size, offset, err)
			}

			if !bytes.Equal(got, full[offset:]) {
				t.Fatalf("size %d offset %d: %d bytes differ from the tail of the whole ciphertext (%d expected)",
					size, offset, len(got), len(full)-int(offset))
			}
		}

		// And the reassembled stream still decrypts.
		meta, plaintext, err := E2EDecrypt(bytes.NewReader(full), key)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if meta.Name != "payload.bin" {
			t.Errorf("metadata name = %q", meta.Name)
		}

		decrypted, err := io.ReadAll(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(decrypted, plain) {
			t.Errorf("size %d: round trip mismatch", size)
		}
	}
}

// A recorded prefix has to rebuild the identical stream, or a resumed upload
// would continue a different ciphertext than the one already on the server.
func TestE2EStreamWithPrefixIsReproducible(t *testing.T) {
	key, err := NewE2EKey()
	if err != nil {
		t.Fatal(err)
	}

	meta := E2EMetadata{Name: "a.log"}

	first, err := NewE2EStream(key, meta)
	if err != nil {
		t.Fatal(err)
	}

	second, err := NewE2EStreamWithPrefix(key, first.Prefix, meta)
	if err != nil {
		t.Fatal(err)
	}

	plain := bytes.Repeat([]byte("log line\n"), 10000)

	a, err := first.Reader(bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Reader(bytes.NewReader(plain))
	if err != nil {
		t.Fatal(err)
	}

	da, _ := io.ReadAll(a)
	db, _ := io.ReadAll(b)

	if !bytes.Equal(da, db) {
		t.Error("the same key and prefix produced different ciphertext")
	}

	if _, err := NewE2EStreamWithPrefix(key, first.Prefix[:3], meta); err == nil {
		t.Error("a short nonce prefix was accepted")
	}
}

func TestPendingRoundTrip(t *testing.T) {
	t.Setenv("SENDTO_CONFIG_DIR", t.TempDir())

	pending, err := LoadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending.Uploads) != 0 {
		t.Fatalf("fresh config dir has %d entries", len(pending.Uploads))
	}

	key := PendingKey("https://send.to", "/tmp/build.tar.gz", 1234, time.Unix(1700000000, 0))

	pending.Put(PendingUpload{Key: key, SessionURL: "https://send.to/upload/abc/build.tar.gz", StartedAt: time.Now()})
	// An expired entry must not survive a save.
	pending.Put(PendingUpload{Key: "stale", SessionURL: "https://send.to/upload/old/x", StartedAt: time.Now().Add(-48 * time.Hour)})

	if err := pending.Save(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := LoadPending()
	if err != nil {
		t.Fatal(err)
	}

	if reloaded.Find(key) == nil {
		t.Error("the recorded session was not reloaded")
	}
	if reloaded.Find("stale") != nil {
		t.Error("an expired session survived")
	}

	// A different file size means a different key, so a changed file cannot be
	// resumed onto the bytes of the old one.
	if PendingKey("https://send.to", "/tmp/build.tar.gz", 1235, time.Unix(1700000000, 0)) == key {
		t.Error("PendingKey ignores the file size")
	}

	reloaded.Remove(key)
	if reloaded.Find(key) != nil {
		t.Error("Remove did not drop the entry")
	}
}
