package auditlog

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestBulkDistinctActorsFor_WithResourceIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT resource_id, actor_id FROM audit_logs").
		WithArgs("acct-1", AgentRegister, "agent", "agent-a", "agent-b").
		WillReturnRows(sqlmock.NewRows([]string{"resource_id", "actor_id"}).
			AddRow("agent-a", "user-1").
			AddRow("agent-a", "user-2").
			AddRow("agent-b", "user-1"))

	s := NewStore(db)
	result, err := s.BulkDistinctActorsFor(context.Background(), "acct-1", AgentRegister, "agent", []string{"agent-a", "agent-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["agent-a"]) != 2 {
		t.Errorf("expected 2 actors for agent-a, got %d", len(result["agent-a"]))
	}
	if result["agent-a"][0] != "user-1" || result["agent-a"][1] != "user-2" {
		t.Errorf("unexpected actors for agent-a: %v", result["agent-a"])
	}
	if len(result["agent-b"]) != 1 || result["agent-b"][0] != "user-1" {
		t.Errorf("unexpected actors for agent-b: %v", result["agent-b"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestLatestPerResourceByAction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	deployedAt := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT DISTINCT ON \\(resource_id\\) resource_id, created_at, actor_id").
		WithArgs("acct-1", DeploymentDeploy, "deployment", "dep-1", "dep-2").
		WillReturnRows(sqlmock.NewRows([]string{"resource_id", "created_at", "actor_id"}).
			AddRow("dep-1", deployedAt, "user-taylor"))

	store := NewStore(db)
	result, err := store.LatestPerResourceByAction(
		context.Background(),
		"acct-1",
		DeploymentDeploy,
		"deployment",
		[]string{"dep-1", "dep-2"},
	)
	if err != nil {
		t.Fatalf("LatestPerResourceByAction: %v", err)
	}
	if got := result["dep-1"].ActorID; got != "user-taylor" {
		t.Errorf("dep-1 actor_id = %q, want %q", got, "user-taylor")
	}
	if _, ok := result["dep-2"]; ok {
		t.Error("dep-2 should have no deployment.deploy entry")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestBulkDistinctActorsFor_NoResourceIDs_ReturnsAll(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT resource_id, actor_id FROM audit_logs").
		WithArgs("acct-1", AgentRegister, "agent").
		WillReturnRows(sqlmock.NewRows([]string{"resource_id", "actor_id"}).
			AddRow("agent-x", "user-1"))

	s := NewStore(db)
	result, err := s.BulkDistinctActorsFor(context.Background(), "acct-1", AgentRegister, "agent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["agent-x"]) != 1 || result["agent-x"][0] != "user-1" {
		t.Errorf("unexpected result: %v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestBulkDistinctActorsFor_EmptyResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT resource_id, actor_id FROM audit_logs").
		WithArgs("acct-1", AgentRegister, "agent").
		WillReturnRows(sqlmock.NewRows([]string{"resource_id", "actor_id"}))

	s := NewStore(db)
	result, err := s.BulkDistinctActorsFor(context.Background(), "acct-1", AgentRegister, "agent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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
