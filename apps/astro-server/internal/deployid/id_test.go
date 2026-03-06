package deployid

import (
	"regexp"
	"testing"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]{3}-[a-z0-9]{3}-[a-z0-9]{3}$`)

func TestNew_Format(t *testing.T) {
	id := New()
	if !idPattern.MatchString(id) {
		t.Errorf("expected xxx-xxx-xxx format, got %q", id)
	}
	if len(id) != 11 {
		t.Errorf("expected length 11, got %d", len(id))
	}
}

func TestNew_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id := New()
		if seen[id] {
			t.Fatalf("collision after %d IDs: %q", i, id)
		}
		seen[id] = true
	}
}

func TestCompact(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"abc-def-ghi", "abcdefghi"},
		{"a1b-2c3-d4e", "a1b2c3d4e"},
	}
	for _, tt := range tests {
		got := Compact(tt.in)
		if got != tt.want {
			t.Errorf("Compact(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNew_AllCharsInAlphabet(t *testing.T) {
	seen := make(map[byte]bool)
	for i := 0; i < 10000; i++ {
		id := New()
		for _, c := range []byte(Compact(id)) {
			seen[c] = true
		}
	}
	// Should see at least most of the 36-char alphabet
	if len(seen) < 30 {
		t.Errorf("expected at least 30 distinct chars, got %d", len(seen))
	}
}
