package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn and returns everything written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	r.Close()
	return string(out)
}

func TestPrintLogs_FormatsEntries(t *testing.T) {
	body := strings.NewReader(`[
		{"timestamp":"2026-04-13T10:00:00Z","level":"INFO","message":"agent started"},
		{"timestamp":"2026-04-13T10:00:01Z","level":"ERROR","message":"connection failed"},
		{"timestamp":"2026-04-13T10:00:02Z","level":"WARN","message":"retry attempt 1"}
	]`)

	out := captureStdout(t, func() {
		if err := printLogs(body); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "2026-04-13T10:00:00Z INFO agent started") {
		t.Errorf("missing INFO line in output:\n%s", out)
	}
	if !strings.Contains(out, "2026-04-13T10:00:01Z ERROR connection failed") {
		t.Errorf("missing ERROR line in output:\n%s", out)
	}
	if !strings.Contains(out, "2026-04-13T10:00:02Z WARN retry attempt 1") {
		t.Errorf("missing WARN line in output:\n%s", out)
	}
}

func TestPrintLogs_EmptyLevelDefaultsToInfo(t *testing.T) {
	body := strings.NewReader(`[{"timestamp":"2026-04-13T10:00:00Z","level":"","message":"no level set"}]`)

	out := captureStdout(t, func() {
		if err := printLogs(body); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "INFO no level set") {
		t.Errorf("expected INFO fallback in output:\n%s", out)
	}
}

func TestPrintLogs_NoTimestampOmitsIt(t *testing.T) {
	body := strings.NewReader(`[{"timestamp":"","level":"INFO","message":"no timestamp"}]`)

	out := captureStdout(t, func() {
		if err := printLogs(body); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(out, "INFO no timestamp") {
		t.Errorf("expected line without timestamp in output:\n%s", out)
	}
}

func TestPrintLogs_EmptyArray(t *testing.T) {
	body := strings.NewReader(`[]`)
	if err := printLogs(body); err != nil {
		t.Fatalf("unexpected error on empty array: %v", err)
	}
}

func TestPrintLogs_InvalidJSON(t *testing.T) {
	body := strings.NewReader(`not json`)
	if err := printLogs(body); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
