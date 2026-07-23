package main

import (
	"strings"
	"testing"
)

func TestReorderArgs(t *testing.T) {
	app := newApp()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			// The regression this exists for: without reordering the limit is
			// dropped and the user gets a permanent link they believed was
			// single-use.
			name: "flags after positionals",
			in:   []string{"send", "put", "a.txt", "--max-downloads", "1", "--quiet"},
			want: []string{"send", "put", "--max-downloads", "1", "--quiet", "--", "a.txt"},
		},
		{
			name: "flags already first are untouched",
			in:   []string{"send", "put", "--days", "7", "a.txt"},
			want: []string{"send", "put", "--days", "7", "--", "a.txt"},
		},
		{
			name: "interleaved",
			in:   []string{"send", "put", "a.txt", "--days", "7", "b.txt", "--quiet"},
			want: []string{"send", "put", "--days", "7", "--quiet", "--", "a.txt", "b.txt"},
		},
		{
			name: "equals form keeps its value",
			in:   []string{"send", "put", "a.txt", "--days=7"},
			want: []string{"send", "put", "--days=7", "--", "a.txt"},
		},
		{
			name: "bool flag does not eat the next argument",
			in:   []string{"send", "put", "--quiet", "a.txt"},
			want: []string{"send", "put", "--quiet", "--", "a.txt"},
		},
		{
			name: "dash alone stays positional",
			in:   []string{"send", "put", "-", "--name", "x.txt"},
			want: []string{"send", "put", "--name", "x.txt", "--", "-"},
		},
		{
			name: "everything after -- is positional",
			in:   []string{"send", "put", "--", "--weird-name.txt"},
			want: []string{"send", "put", "--", "--weird-name.txt"},
		},
		{
			name: "aliases resolve to the same command",
			in:   []string{"send", "up", "a.txt", "--days", "3"},
			want: []string{"send", "up", "--days", "3", "--", "a.txt"},
		},
		{
			name: "nested subcommands",
			in:   []string{"send", "config", "add", "work", "https://x.example", "--default"},
			want: []string{"send", "config", "add", "--default", "--", "work", "https://x.example"},
		},
		{
			name: "no arguments",
			in:   []string{"send"},
			want: []string{"send"},
		},
		{
			name: "unknown command is left alone",
			in:   []string{"send", "bogus", "--flag"},
			want: []string{"send", "--flag", "--", "bogus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderArgs(app, tt.in)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Errorf("reorderArgs(%v)\n got %v\nwant %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRejectFlagLikeArgs(t *testing.T) {
	if err := rejectFlagLikeArgs([]string{"a.txt", "-", "b.txt"}); err != nil {
		t.Errorf("valid arguments rejected: %v", err)
	}

	err := rejectFlagLikeArgs([]string{"a.txt", "--typo"})
	if err == nil {
		t.Fatal("a flag-looking argument should be rejected, not uploaded as a file")
	}
	if !strings.Contains(err.Error(), "--typo") {
		t.Errorf("error should name the offending argument: %v", err)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{
		0:          "0 B",
		999:        "999 B",
		1024:       "1.0 KB",
		1536:       "1.5 KB",
		1048576:    "1.0 MB",
		1073741824: "1.0 GB",
		-1:         "?",
	}

	for in, want := range tests {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
