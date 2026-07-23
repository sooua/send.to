package client

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
)

func mustKey(t *testing.T) E2EKey {
	t.Helper()
	key, err := NewE2EKey()
	if err != nil {
		t.Fatalf("NewE2EKey: %v", err)
	}
	return key
}

func encryptToBytes(t *testing.T, plaintext []byte, key E2EKey, meta E2EMetadata) []byte {
	t.Helper()
	r, err := E2EEncrypt(bytes.NewReader(plaintext), key, meta)
	if err != nil {
		t.Fatalf("E2EEncrypt: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading ciphertext: %v", err)
	}
	return out
}

// Chunk boundaries are where a streaming AEAD breaks, so cover them exactly.
func TestE2ERoundTripAcrossChunkBoundaries(t *testing.T) {
	sizes := []int{
		0,
		1,
		e2eChunkSize - 1,
		e2eChunkSize,
		e2eChunkSize + 1,
		2 * e2eChunkSize,
		2*e2eChunkSize + 7,
	}

	for _, size := range sizes {
		t.Run(byteCountName(size), func(t *testing.T) {
			plaintext := make([]byte, size)
			if _, err := rand.Read(plaintext); err != nil {
				t.Fatal(err)
			}

			key := mustKey(t)
			meta := E2EMetadata{Name: "report.pdf", Type: "application/pdf"}

			ciphertext := encryptToBytes(t, plaintext, key, meta)

			if bytes.Contains(ciphertext, plaintext) && size > 32 {
				t.Error("plaintext appears verbatim in the ciphertext")
			}

			gotMeta, r, err := E2EDecrypt(bytes.NewReader(ciphertext), key)
			if err != nil {
				t.Fatalf("E2EDecrypt: %v", err)
			}
			if gotMeta != meta {
				t.Errorf("metadata = %+v, want %+v", gotMeta, meta)
			}

			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("reading plaintext: %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Errorf("round trip mismatch at %d bytes (got %d)", size, len(got))
			}

			// The declared length must match, or an upload's Content-Length
			// would be wrong.
			want, err := E2EEncryptedSize(int64(size), meta)
			if err != nil {
				t.Fatal(err)
			}
			if want != int64(len(ciphertext)) {
				t.Errorf("E2EEncryptedSize = %d, actual ciphertext = %d", want, len(ciphertext))
			}
		})
	}
}

func TestE2EWrongKeyFails(t *testing.T) {
	plaintext := []byte("the quick brown fox")
	ciphertext := encryptToBytes(t, plaintext, mustKey(t), E2EMetadata{Name: "a.txt"})

	_, _, err := E2EDecrypt(bytes.NewReader(ciphertext), mustKey(t))
	if !errors.Is(err, ErrWrongKey) {
		t.Fatalf("err = %v, want ErrWrongKey", err)
	}
}

func TestE2ETamperingIsDetected(t *testing.T) {
	key := mustKey(t)
	plaintext := bytes.Repeat([]byte("A"), 3*e2eChunkSize)
	ciphertext := encryptToBytes(t, plaintext, key, E2EMetadata{Name: "a.bin"})

	t.Run("flipped bit in the payload", func(t *testing.T) {
		corrupt := append([]byte(nil), ciphertext...)
		corrupt[len(corrupt)/2] ^= 0x01

		_, r, err := E2EDecrypt(bytes.NewReader(corrupt), key)
		if err == nil {
			_, err = io.ReadAll(r)
		}
		if err == nil {
			t.Fatal("a modified payload decrypted successfully")
		}
	})

	t.Run("flipped bit in the header", func(t *testing.T) {
		corrupt := append([]byte(nil), ciphertext...)
		corrupt[e2eHeaderSize-1] ^= 0x01 // nonce prefix

		_, _, err := E2EDecrypt(bytes.NewReader(corrupt), key)
		if err == nil {
			t.Fatal("a modified nonce prefix decrypted successfully")
		}
	})

	// The whole reason the final marker lives in the nonce: lopping off the
	// tail must not read back as a shorter, valid file.
	t.Run("truncated tail", func(t *testing.T) {
		truncated := ciphertext[:len(ciphertext)-e2eChunkCipherSize]

		_, r, err := E2EDecrypt(bytes.NewReader(truncated), key)
		if err == nil {
			_, err = io.ReadAll(r)
		}
		if err == nil {
			t.Fatal("a truncated stream decrypted successfully")
		}
	})

	t.Run("swapped chunks", func(t *testing.T) {
		corrupt := append([]byte(nil), ciphertext...)
		start := e2eHeaderSize + 34 + e2eTagSize // past the metadata chunk

		if len(corrupt) < start+2*e2eChunkCipherSize {
			t.Skip("ciphertext too short for this case")
		}

		first := append([]byte(nil), corrupt[start:start+e2eChunkCipherSize]...)
		second := append([]byte(nil), corrupt[start+e2eChunkCipherSize:start+2*e2eChunkCipherSize]...)
		copy(corrupt[start:], second)
		copy(corrupt[start+e2eChunkCipherSize:], first)

		_, r, err := E2EDecrypt(bytes.NewReader(corrupt), key)
		if err == nil {
			_, err = io.ReadAll(r)
		}
		if err == nil {
			t.Fatal("reordered chunks decrypted successfully")
		}
	})
}

func TestE2ERejectsForeignData(t *testing.T) {
	_, _, err := E2EDecrypt(strings.NewReader("just some plain text, not encrypted at all"), mustKey(t))
	if err == nil {
		t.Fatal("plain data was accepted as an encrypted stream")
	}
	if !strings.Contains(err.Error(), "end-to-end encrypted") {
		t.Errorf("error should say the file is not encrypted, got %v", err)
	}
}

func TestE2EKeyEncoding(t *testing.T) {
	key := mustKey(t)

	encoded := key.String()
	if strings.ContainsAny(encoded, "+/=") {
		t.Errorf("key encoding %q is not URL-safe", encoded)
	}

	parsed, err := ParseE2EKey(encoded)
	if err != nil {
		t.Fatalf("ParseE2EKey: %v", err)
	}
	if !bytes.Equal(parsed, key) {
		t.Error("key did not survive the round trip")
	}

	for _, bad := range []string{"", "short", strings.Repeat("A", 100), "!!!not base64!!!"} {
		if _, err := ParseE2EKey(bad); err == nil {
			t.Errorf("ParseE2EKey(%q) should have failed", bad)
		}
	}
}

func TestFragmentKey(t *testing.T) {
	key := mustKey(t)
	base := "https://send.to/aB3cD4eF/report.pdf"

	t.Run("k= form", func(t *testing.T) {
		got, err := FragmentKey(base + "#k=" + key.String())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, key) {
			t.Error("key mismatch")
		}
	})

	t.Run("bare form", func(t *testing.T) {
		got, err := FragmentKey(base + "#" + key.String())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, key) {
			t.Error("key mismatch")
		}
	})

	t.Run("no fragment", func(t *testing.T) {
		got, err := FragmentKey(base)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Error("expected no key")
		}
	})

	t.Run("malformed fragment is an error", func(t *testing.T) {
		if _, err := FragmentKey(base + "#k=nope"); err == nil {
			t.Error("expected an error")
		}
	})

	// The key must never reach the server.
	t.Run("strip", func(t *testing.T) {
		if got := StripFragment(base + "#k=" + key.String()); got != base {
			t.Errorf("StripFragment = %q, want %q", got, base)
		}
		if got := StripFragment(base); got != base {
			t.Errorf("StripFragment = %q, want %q", got, base)
		}
	})
}

// Two encryptions of the same input under the same key must differ: the nonce
// prefix is random, so a repeated upload is not linkable by its ciphertext.
func TestE2ENonceIsFresh(t *testing.T) {
	key := mustKey(t)
	plaintext := []byte("same input every time")
	meta := E2EMetadata{Name: "a.txt"}

	first := encryptToBytes(t, plaintext, key, meta)
	second := encryptToBytes(t, plaintext, key, meta)

	if bytes.Equal(first, second) {
		t.Fatal("two encryptions produced identical ciphertext")
	}
}

func byteCountName(n int) string {
	switch {
	case n == 0:
		return "empty"
	case n < 1024:
		return "small"
	case n == e2eChunkSize:
		return "exactly-one-chunk"
	case n == e2eChunkSize-1:
		return "one-chunk-minus-one"
	case n == e2eChunkSize+1:
		return "one-chunk-plus-one"
	case n == 2*e2eChunkSize:
		return "exactly-two-chunks"
	default:
		return "multi-chunk"
	}
}
