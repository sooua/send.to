package server

import (
	"bytes"
	"io"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plaintext := "Hello, this is a secret message for testing encryption!"
	password := "test-password-123"

	// Encrypt
	reader := io.NopCloser(bytes.NewReader([]byte(plaintext)))
	encrypted, err := attachEncryptionReader(reader, password)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	encryptedData, err := io.ReadAll(encrypted)
	if err != nil {
		t.Fatalf("reading encrypted data failed: %v", err)
	}

	// Encrypted data should differ from plaintext
	if string(encryptedData) == plaintext {
		t.Error("encrypted data should differ from plaintext")
	}

	// Decrypt
	encReader := io.NopCloser(bytes.NewReader(encryptedData))
	decrypted, err := attachDecryptionReader(encReader, password)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	decryptedData, err := io.ReadAll(decrypted)
	if err != nil {
		t.Fatalf("reading decrypted data failed: %v", err)
	}

	if string(decryptedData) != plaintext {
		t.Errorf("decrypted data = %q, want %q", string(decryptedData), plaintext)
	}
}

func TestEncryptDecryptEmptyPassword(t *testing.T) {
	plaintext := "no encryption here"
	reader := io.NopCloser(bytes.NewReader([]byte(plaintext)))

	// Empty password should pass through unchanged
	result, err := attachEncryptionReader(reader, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := io.ReadAll(result)
	if err != nil {
		t.Fatalf("reading data failed: %v", err)
	}

	if string(data) != plaintext {
		t.Errorf("data = %q, want %q", string(data), plaintext)
	}
}

func TestDecryptEmptyPassword(t *testing.T) {
	plaintext := "no decryption here"
	reader := io.NopCloser(bytes.NewReader([]byte(plaintext)))

	result, err := attachDecryptionReader(reader, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := io.ReadAll(result)
	if err != nil {
		t.Fatalf("reading data failed: %v", err)
	}

	if string(data) != plaintext {
		t.Errorf("data = %q, want %q", string(data), plaintext)
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	plaintext := "secret data"
	password := "correct-password"

	// Encrypt
	reader := io.NopCloser(bytes.NewReader([]byte(plaintext)))
	encrypted, err := attachEncryptionReader(reader, password)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	encryptedData, err := io.ReadAll(encrypted)
	if err != nil {
		t.Fatalf("reading encrypted data failed: %v", err)
	}

	// Decrypt with wrong password
	encReader := io.NopCloser(bytes.NewReader(encryptedData))
	_, err = attachDecryptionReader(encReader, "wrong-password")
	if err == nil {
		t.Error("expected error when decrypting with wrong password")
	}
}

func TestEncryptDecryptBinaryData(t *testing.T) {
	// Test with binary data (non-UTF8)
	plaintext := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD, 0x80, 0x90}
	password := "binary-password"

	reader := io.NopCloser(bytes.NewReader(plaintext))
	encrypted, err := attachEncryptionReader(reader, password)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	encryptedData, err := io.ReadAll(encrypted)
	if err != nil {
		t.Fatalf("reading encrypted data failed: %v", err)
	}

	encReader := io.NopCloser(bytes.NewReader(encryptedData))
	decrypted, err := attachDecryptionReader(encReader, password)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	decryptedData, err := io.ReadAll(decrypted)
	if err != nil {
		t.Fatalf("reading decrypted data failed: %v", err)
	}

	if !bytes.Equal(decryptedData, plaintext) {
		t.Errorf("decrypted binary data does not match original")
	}
}
