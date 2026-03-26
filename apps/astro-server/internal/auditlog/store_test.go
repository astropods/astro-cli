package auditlog

import (
	"testing"
	"time"
)

func TestParseCursor_Empty(t *testing.T) {
	ts, id := ParseCursor("")
	if ts != nil {
		t.Errorf("expected nil timestamp, got %v", ts)
	}
	if id != 0 {
		t.Errorf("expected 0 id, got %d", id)
	}
}

func TestParseCursor_TimestampOnly(t *testing.T) {
	input := "2026-03-26T10:00:00.123456789Z"
	ts, id := ParseCursor(input)
	if ts == nil {
		t.Fatal("expected non-nil timestamp")
	}
	expected, _ := time.Parse(time.RFC3339Nano, input)
	if !ts.Equal(expected) {
		t.Errorf("timestamp = %v, want %v", *ts, expected)
	}
	if id != 0 {
		t.Errorf("expected 0 id for timestamp-only cursor, got %d", id)
	}
}

func TestParseCursor_Composite(t *testing.T) {
	input := "2026-03-26T10:00:00.123456789Z,42"
	ts, id := ParseCursor(input)
	if ts == nil {
		t.Fatal("expected non-nil timestamp")
	}
	expected, _ := time.Parse(time.RFC3339Nano, "2026-03-26T10:00:00.123456789Z")
	if !ts.Equal(expected) {
		t.Errorf("timestamp = %v, want %v", *ts, expected)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}

func TestParseCursor_InvalidTimestamp(t *testing.T) {
	ts, id := ParseCursor("not-a-timestamp,42")
	if ts != nil {
		t.Errorf("expected nil timestamp for invalid input, got %v", ts)
	}
	if id != 0 {
		t.Errorf("expected 0 id for invalid input, got %d", id)
	}
}

func TestFormatCursor_RoundTrip(t *testing.T) {
	now := time.Now().UTC()
	entry := Entry{
		ID:        123,
		CreatedAt: now,
	}
	cursor := FormatCursor(entry)
	ts, id := ParseCursor(cursor)
	if ts == nil {
		t.Fatal("expected non-nil timestamp after round-trip")
	}
	// Timestamps may lose sub-nanosecond precision, compare within tolerance
	if ts.Sub(now).Abs() > time.Microsecond {
		t.Errorf("timestamp drift: got %v, want %v", *ts, now)
	}
	if id != 123 {
		t.Errorf("id = %d, want 123", id)
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		input    string
		def, max int
		want     int
	}{
		{"", 50, 200, 50},
		{"10", 50, 200, 10},
		{"0", 50, 200, 50},
		{"-1", 50, 200, 50},
		{"999", 50, 200, 200},
		{"abc", 50, 200, 50},
	}
	for _, tt := range tests {
		got := ParseLimit(tt.input, tt.def, tt.max)
		if got != tt.want {
			t.Errorf("ParseLimit(%q, %d, %d) = %d, want %d", tt.input, tt.def, tt.max, got, tt.want)
		}
	}
}
