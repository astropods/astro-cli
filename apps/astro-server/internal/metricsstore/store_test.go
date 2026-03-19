package metricsstore

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBulkMessageCounts(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)

	mock.ExpectQuery("SELECT agent_name, lifetime_total FROM agent_message_counts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "lifetime_total"}).
			AddRow("agent-a", 100).
			AddRow("agent-b", 250))

	counts, err := s.BulkMessageCounts("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["agent-a"] != 100 {
		t.Errorf("agent-a = %d, want 100", counts["agent-a"])
	}
	if counts["agent-b"] != 250 {
		t.Errorf("agent-b = %d, want 250", counts["agent-b"])
	}
}

func TestBulkMessageCounts_Empty(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)

	mock.ExpectQuery("SELECT agent_name, lifetime_total FROM agent_message_counts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "lifetime_total"}))

	counts, err := s.BulkMessageCounts("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty map, got %d entries", len(counts))
	}
}

func TestBulkMessageCounts_NilStore(t *testing.T) {
	var s *Store
	counts, err := s.BulkMessageCounts("acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts != nil {
		t.Error("expected nil counts for nil store")
	}
}

func TestMessageCount(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)

	mock.ExpectQuery("SELECT lifetime_total FROM agent_message_counts").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"lifetime_total"}).AddRow(42))

	total, err := s.MessageCount("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}
}

func TestMessageCount_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)

	mock.ExpectQuery("SELECT lifetime_total FROM agent_message_counts").
		WithArgs("acct-1", "missing-agent").
		WillReturnRows(sqlmock.NewRows([]string{"lifetime_total"}))

	total, err := s.MessageCount("acct-1", "missing-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 for missing agent", total)
	}
}

func TestMessageCount_NilStore(t *testing.T) {
	var s *Store
	total, err := s.MessageCount("acct-1", "my-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 for nil store", total)
	}
}
