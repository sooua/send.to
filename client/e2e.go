package client

// End-to-end encryption.
//
// The server's X-Encrypt-Password option encrypts at rest, but the server sees
// the plaintext and the password on the way through. This does not: the key is
// generated on the client, never leaves it, and travels in the URL *fragment*
// — the one part of a URL that browsers, proxies and access logs never see,
// because it is stripped before the request is sent.
//
// Wire format, identical in Go and in the browser (see web/src/lib/e2e.ts):
//
//	"STE1"                       4 bytes  magic and version
//	nonce prefix                 7 bytes  random
//	metadata length              2 bytes  big-endian, ciphertext incl. tag
//	chunk 0                    variable   AES-256-GCM(metadata JSON)
//	chunk 1..n                            AES-256-GCM(plaintext chunk)
//
// Each chunk is at most 64 KiB of plaintext plus a 16-byte tag. The nonce is
// the prefix, a big-endian chunk counter, and a final-chunk marker:
//
//	nonce = prefix[0:7] || counter[4] || (0x01 on the last chunk else 0x00)
//
// Folding the marker into the nonce is what makes truncation detectable: a
// stream cut short cannot produce a chunk that opens under a final nonce, so
// dropping the tail is an authentication failure rather than a silent partial
// file. This is the STREAM construction, as used by age.
//
// AES-GCM rather than ChaCha20-Poly1305 because WebCrypto implements it
// natively in every browser, and the browser has to be able to decrypt.

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// e2eMagic identifies the format and its version.
	e2eMagic = "STE1"

	// e2eKeySize is 32 bytes: AES-256.
	e2eKeySize = 32

	// e2eNoncePrefixSize leaves room for the counter and final marker in a
	// 12-byte GCM nonce.
	e2eNoncePrefixSize = 7

	// e2eChunkSize is the plaintext size of one chunk. 64 KiB keeps the
	// working set small enough to stream a file of any size.
	e2eChunkSize = 64 * 1024

	// e2eTagSize is the AES-GCM authentication tag.
	e2eTagSize = 16

	// e2eMetaLenSize prefixes the variable-length metadata chunk. Without it
	// a reader cannot tell where the metadata ends and the payload begins.
	e2eMetaLenSize = 2

	e2eHeaderSize      = len(e2eMagic) + e2eNoncePrefixSize + e2eMetaLenSize
	e2eChunkCipherSize = e2eChunkSize + e2eTagSize
)

// ErrWrongKey means the payload did not authenticate: the wrong key, or a
// modified or truncated file. The two are deliberately indistinguishable.
var ErrWrongKey = errors.New("could not decrypt — wrong key, or the file was modified or truncated")

// E2EMetadata is the small header encrypted alongside the payload, so the real
// filename and type need not be exposed in the URL.
type E2EMetadata struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// E2EKey is a content encryption key.
type E2EKey []byte

// NewE2EKey returns a fresh random key.
func NewE2EKey() (E2EKey, error) {
	key := make([]byte, e2eKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("could not generate a key: %w", err)
	}
	return key, nil
}

// String renders the key for a URL fragment: unpadded base64url, so it stays
// URL-safe and copy-pasteable.
func (k E2EKey) String() string {
	return base64.RawURLEncoding.EncodeToString(k)
}

// ParseE2EKey reads a key from its fragment encoding.
func ParseE2EKey(s string) (E2EKey, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("malformed key: %w", err)
	}
	if len(key) != e2eKeySize {
		return nil, fmt.Errorf("malformed key: got %d bytes, want %d", len(key), e2eKeySize)
	}
	return key, nil
}

// FragmentKey extracts the key from a share URL's fragment, returning nil when
// there is none. Accepts both `#k=<key>` and a bare `#<key>`.
func FragmentKey(rawURL string) (E2EKey, error) {
	_, fragment, found := strings.Cut(rawURL, "#")
	if !found || fragment == "" {
		return nil, nil
	}

	fragment = strings.TrimPrefix(fragment, "k=")

	return ParseE2EKey(fragment)
}

// StripFragment returns the URL without its fragment. A fragment is never sent
// to a server, but strip it explicitly so the key cannot leak through a
// mis-built request.
func StripFragment(rawURL string) string {
	base, _, _ := strings.Cut(rawURL, "#")
	return base
}

func e2eNonce(prefix []byte, counter uint32, final bool) []byte {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[e2eNoncePrefixSize:], counter)
	if final {
		nonce[11] = 1
	}
	return nonce
}

func e2eAEAD(key E2EKey) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// E2EStream is one encryption of one file. Because the ciphertext is a pure
// function of the key, the nonce prefix and the plaintext, a stream can
// regenerate its own bytes from any offset — which is what makes an interrupted
// client-side-encrypted upload resumable rather than a restart from zero.
type E2EStream struct {
	Key    E2EKey
	Prefix []byte
	Meta   E2EMetadata
}

// NewE2EStream starts an encryption with a fresh random nonce prefix.
func NewE2EStream(key E2EKey, meta E2EMetadata) (*E2EStream, error) {
	prefix := make([]byte, e2eNoncePrefixSize)
	if _, err := rand.Read(prefix); err != nil {
		return nil, fmt.Errorf("could not generate a nonce: %w", err)
	}

	return NewE2EStreamWithPrefix(key, prefix, meta)
}

// NewE2EStreamWithPrefix rebuilds a stream from a prefix recorded earlier, so a
// resumed upload continues the ciphertext the server already holds instead of
// starting a different one.
func NewE2EStreamWithPrefix(key E2EKey, prefix []byte, meta E2EMetadata) (*E2EStream, error) {
	if len(key) != e2eKeySize {
		return nil, fmt.Errorf("malformed key: got %d bytes, want %d", len(key), e2eKeySize)
	}
	if len(prefix) != e2eNoncePrefixSize {
		return nil, fmt.Errorf("malformed nonce prefix: got %d bytes, want %d", len(prefix), e2eNoncePrefixSize)
	}

	return &E2EStream{Key: key, Prefix: prefix, Meta: meta}, nil
}

// header is the fixed part of the format plus the encrypted metadata chunk.
func (s *E2EStream) header() ([]byte, error) {
	aead, err := e2eAEAD(s.Key)
	if err != nil {
		return nil, err
	}

	metaJSON, err := json.Marshal(s.Meta)
	if err != nil {
		return nil, err
	}
	if len(metaJSON) > e2eChunkSize {
		return nil, errors.New("metadata is too large")
	}

	// Chunk 0 is the metadata; the payload starts at 1.
	sealedMeta := aead.Seal(nil, e2eNonce(s.Prefix, 0, false), metaJSON, nil)
	if len(sealedMeta) > 0xFFFF {
		return nil, errors.New("metadata is too large")
	}

	header := append([]byte(e2eMagic), s.Prefix...)
	header = binary.BigEndian.AppendUint16(header, uint16(len(sealedMeta)))

	return append(header, sealedMeta...), nil
}

// Size returns the exact ciphertext length for a plaintext of n bytes.
func (s *E2EStream) Size(n int64) (int64, error) {
	return E2EEncryptedSize(n, s.Meta)
}

// Reader returns the whole ciphertext, encrypting as it is consumed so memory
// use is one chunk regardless of file size.
func (s *E2EStream) Reader(plaintext io.Reader) (io.Reader, error) {
	header, err := s.header()
	if err != nil {
		return nil, err
	}

	return s.chunks(plaintext, header, 1)
}

// ReaderFrom returns the ciphertext from byte offset onwards. The plaintext is
// rewound to the chunk the offset falls in and the leading bytes of that chunk
// are discarded, so any offset works — not only chunk boundaries.
func (s *E2EStream) ReaderFrom(plaintext io.ReadSeeker, offset int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, errors.New("negative offset")
	}

	header, err := s.header()
	if err != nil {
		return nil, err
	}

	headerLen := int64(len(header))

	var (
		counter     = uint32(1)
		plainOffset int64
		skip        = offset
	)

	if offset >= headerLen {
		index := (offset - headerLen) / e2eChunkCipherSize
		counter = uint32(index) + 1
		plainOffset = index * e2eChunkSize
		skip = (offset - headerLen) - index*e2eChunkCipherSize
		header = nil
	}

	if _, err := plaintext.Seek(plainOffset, io.SeekStart); err != nil {
		return nil, err
	}

	reader, err := s.chunks(plaintext, header, counter)
	if err != nil {
		return nil, err
	}

	if skip > 0 {
		if _, err := io.CopyN(io.Discard, reader, skip); err != nil {
			_ = reader.Close()
			return nil, err
		}
	}

	return reader, nil
}

// chunks writes header (which may be empty) and then every chunk from counter
// onwards into a pipe. Closing the returned reader stops the goroutine.
func (s *E2EStream) chunks(plaintext io.Reader, header []byte, counter uint32) (io.ReadCloser, error) {
	aead, err := e2eAEAD(s.Key)
	if err != nil {
		return nil, err
	}

	prefix := s.Prefix
	pr, pw := io.Pipe()

	go func() {
		if len(header) > 0 {
			if _, err := pw.Write(header); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}

		buf := make([]byte, e2eChunkSize)
		reader := bufio.NewReaderSize(plaintext, e2eChunkSize)

		for ; ; counter++ {
			n, readErr := io.ReadFull(reader, buf)

			final := false
			switch {
			case errors.Is(readErr, io.EOF), errors.Is(readErr, io.ErrUnexpectedEOF):
				// Short or empty read: nothing follows.
				final = true
			case readErr != nil:
				_ = pw.CloseWithError(readErr)
				return
			default:
				// A full chunk may still be the last one; look ahead.
				if _, peekErr := reader.Peek(1); errors.Is(peekErr, io.EOF) {
					final = true
				}
			}

			sealed := aead.Seal(nil, e2eNonce(prefix, counter, final), buf[:n], nil)
			if _, err := pw.Write(sealed); err != nil {
				_ = pw.CloseWithError(err)
				return
			}

			if final {
				break
			}
		}

		_ = pw.Close()
	}()

	return pr, nil
}

// E2EEncrypt returns a reader over the encrypted stream. Encryption happens as
// the reader is consumed, so memory use is one chunk regardless of file size.
func E2EEncrypt(plaintext io.Reader, key E2EKey, meta E2EMetadata) (io.Reader, error) {
	stream, err := NewE2EStream(key, meta)
	if err != nil {
		return nil, err
	}

	return stream.Reader(plaintext)
}

// E2EEncryptedSize returns the exact ciphertext length for a plaintext of size
// n with the given metadata, so an upload can still declare a Content-Length.
func E2EEncryptedSize(n int64, meta E2EMetadata) (int64, error) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return 0, err
	}

	size := int64(e2eHeaderSize) + int64(len(metaJSON)+e2eTagSize)

	// A zero-length payload still produces one (empty, final) chunk.
	full := n / e2eChunkSize
	rest := n % e2eChunkSize

	size += full * int64(e2eChunkCipherSize)
	if rest > 0 || full == 0 {
		size += rest + int64(e2eTagSize)
	}

	return size, nil
}

// E2EDecrypt authenticates and decrypts a stream, returning its metadata and a
// reader over the plaintext.
func E2EDecrypt(ciphertext io.Reader, key E2EKey) (E2EMetadata, io.Reader, error) {
	var meta E2EMetadata

	aead, err := e2eAEAD(key)
	if err != nil {
		return meta, nil, err
	}

	reader := bufio.NewReaderSize(ciphertext, e2eChunkCipherSize)

	header := make([]byte, e2eHeaderSize)
	if _, err := io.ReadFull(reader, header); err != nil {
		return meta, nil, errors.New("not an end-to-end encrypted file")
	}
	if string(header[:len(e2eMagic)]) != e2eMagic {
		return meta, nil, errors.New("not an end-to-end encrypted file (or a newer format)")
	}

	prefix := header[len(e2eMagic) : len(e2eMagic)+e2eNoncePrefixSize]
	metaLen := binary.BigEndian.Uint16(header[len(e2eMagic)+e2eNoncePrefixSize:])

	// Decrypt the metadata up front, so a wrong key is reported before the
	// caller starts writing a file rather than halfway through it.
	metaBuf := make([]byte, metaLen)
	if _, err := io.ReadFull(reader, metaBuf); err != nil {
		return meta, nil, ErrWrongKey
	}

	metaJSON, err := aead.Open(nil, e2eNonce(prefix, 0, false), metaBuf, nil)
	if err != nil {
		return meta, nil, ErrWrongKey
	}
	if err := json.Unmarshal(metaJSON, &meta); err != nil {
		return meta, nil, ErrWrongKey
	}

	return meta, &e2eReader{aead: aead, prefix: prefix, reader: reader, counter: 1}, nil
}

// e2eReader decrypts chunk by chunk as it is read.
type e2eReader struct {
	aead    cipher.AEAD
	prefix  []byte
	reader  *bufio.Reader
	counter uint32

	pending []byte
	done    bool
	err     error
}

func (r *e2eReader) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		if r.done {
			return 0, io.EOF
		}
		if err := r.next(); err != nil {
			r.err = err
			return 0, err
		}
	}

	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *e2eReader) next() error {
	buf := make([]byte, e2eChunkCipherSize)
	n, readErr := io.ReadFull(r.reader, buf)

	final := false
	switch {
	case errors.Is(readErr, io.EOF):
		// The stream ended without a chunk carrying the final marker.
		return ErrWrongKey
	case errors.Is(readErr, io.ErrUnexpectedEOF):
		final = true
	case readErr != nil:
		return readErr
	default:
		if _, peekErr := r.reader.Peek(1); errors.Is(peekErr, io.EOF) {
			final = true
		}
	}

	plain, err := r.aead.Open(nil, e2eNonce(r.prefix, r.counter, final), buf[:n], nil)
	if err != nil {
		return ErrWrongKey
	}

	r.counter++
	r.pending = plain
	r.done = final

	return nil
}
