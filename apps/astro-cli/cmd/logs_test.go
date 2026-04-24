package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintLogs_FormatsEntries(t *testing.T) {
	body := strings.NewReader(`[
		{"timestamp":"2026-04-13T10:00:00Z","level":"INFO","message":"agent started"},
		{"timestamp":"2026-04-13T10:00:01Z","level":"ERROR","message":"connection failed"},
		{"timestamp":"2026-04-13T10:00:02Z","level":"WARN","message":"retry attempt 1"}
	]`)

	var buf bytes.Buffer
	if err := printLogs(&buf, body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

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

	var buf bytes.Buffer
	if err := printLogs(&buf, body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "INFO no level set") {
		t.Errorf("expected INFO fallback in output:\n%s", buf.String())
	}
}

func TestPrintLogs_NoTimestampOmitsIt(t *testing.T) {
	body := strings.NewReader(`[{"timestamp":"","level":"INFO","message":"no timestamp"}]`)

	var buf bytes.Buffer
	if err := printLogs(&buf, body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "INFO no timestamp") {
		t.Errorf("expected line without timestamp in output:\n%s", buf.String())
	}
}

func TestPrintLogs_EmptyArray(t *testing.T) {
	body := strings.NewReader(`[]`)
	var buf bytes.Buffer
	if err := printLogs(&buf, body); err != nil {
		t.Fatalf("unexpected error on empty array: %v", err)
	}
	if !strings.Contains(buf.String(), "No logs found") {
		t.Errorf("expected 'No logs found' message for empty array, got:\n%s", buf.String())
	}
}

func TestPrintLogs_InvalidJSON(t *testing.T) {
	body := strings.NewReader(`not json`)
	var buf bytes.Buffer
	if err := printLogs(&buf, body); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
