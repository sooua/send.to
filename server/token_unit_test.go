package server

import (
	"strings"
	"testing"
)

func TestTokenLength(t *testing.T) {
	for _, length := range []int{1, 5, 10, 20, 50} {
		tok := token(length)
		if len(tok) != length {
			t.Errorf("token(%d) returned length %d", length, len(tok))
		}
	}
}

func TestTokenCharacterSet(t *testing.T) {
	for i := 0; i < 100; i++ {
		tok := token(20)
		for _, c := range tok {
			if !strings.ContainsRune(SYMBOLS, c) {
				t.Errorf("token contains invalid character: %c", c)
			}
		}
	}
}

func TestTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		tok := token(10)
		if seen[tok] {
			t.Errorf("duplicate token generated: %s", tok)
		}
		seen[tok] = true
	}
}

func TestTokenZeroLength(t *testing.T) {
	tok := token(0)
	if tok != "" {
		t.Errorf("token(0) should return empty string, got %q", tok)
	}
}

func TestTokenIncludesAllCharacterRanges(t *testing.T) {
	// Generate many tokens and verify we see digits, lowercase, uppercase
	hasDigit := false
	hasLower := false
	hasUpper := false

	for i := 0; i < 100; i++ {
		tok := token(20)
		for _, c := range tok {
			if c >= '0' && c <= '9' {
				hasDigit = true
			}
			if c >= 'a' && c <= 'z' {
				hasLower = true
			}
			if c >= 'A' && c <= 'Z' {
				hasUpper = true
			}
		}
	}

	if !hasDigit {
		t.Error("no digits found in 100 tokens of length 20")
	}
	if !hasLower {
		t.Error("no lowercase found in 100 tokens of length 20")
	}
	if !hasUpper {
		t.Error("no uppercase found in 100 tokens of length 20")
	}
}
