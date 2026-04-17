package clamd

import "testing"

func TestParseResultOK(t *testing.T) {
	r := parseResult("/tmp/foo: OK")
	if r.Status != ResOK {
		t.Fatalf("status = %q want OK", r.Status)
	}
	if r.Path != "/tmp/foo" {
		t.Errorf("path = %q want /tmp/foo", r.Path)
	}
}

func TestParseResultFound(t *testing.T) {
	r := parseResult("/tmp/eicar: Eicar-Test-Signature FOUND")
	if r.Status != ResFound {
		t.Fatalf("status = %q want FOUND", r.Status)
	}
	if r.Description == "" {
		t.Error("description empty")
	}
}

func TestParseResultMalformed(t *testing.T) {
	r := parseResult("totally-garbage")
	if r.Status != ResParseError {
		t.Fatalf("status = %q want PARSE ERROR", r.Status)
	}
}

func TestNewClamdNoPanic(t *testing.T) {
	// Just ensure the constructor doesn't touch the network.
	c := NewClamd("tcp://127.0.0.1:3310")
	if c == nil {
		t.Fatal("NewClamd returned nil")
	}
}
