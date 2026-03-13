package heartstore

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHeart_New(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("INSERT INTO agent_hearts").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"bool"}).AddRow(true))

	created, err := s.Heart(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for new heart")
	}
}

func TestHeart_Idempotent(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	// ON CONFLICT DO NOTHING returns no rows
	mock.ExpectQuery("INSERT INTO agent_hearts").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"bool"}))

	created, err := s.Heart(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false for duplicate heart")
	}
}

func TestUnheart_Exists(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM agent_hearts").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	removed, err := s.Unheart(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !removed {
		t.Error("expected removed=true")
	}
}

func TestUnheart_NotExists(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectExec("DELETE FROM agent_hearts").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	removed, err := s.Unheart(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed {
		t.Error("expected removed=false when no row existed")
	}
}

func TestCount(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("acct-1", "my-agent").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	count, err := s.Count(ctx, "acct-1", "my-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 42 {
		t.Errorf("expected 42, got %d", count)
	}
}

func TestIsHearted(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	hearted, err := s.IsHearted(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hearted {
		t.Error("expected hearted=true")
	}
}

func TestInfo(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count", "exists"}).AddRow(5, true))

	info, err := s.Info(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Count != 5 {
		t.Errorf("expected count=5, got %d", info.Count)
	}
	if !info.Hearted {
		t.Error("expected hearted=true")
	}
}

func TestInfo_NotHearted(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT").
		WithArgs("acct-1", "my-agent", "").
		WillReturnRows(sqlmock.NewRows([]string{"count", "exists"}).AddRow(3, false))

	info, err := s.Info(ctx, "acct-1", "my-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Count != 3 {
		t.Errorf("expected count=3, got %d", info.Count)
	}
	if info.Hearted {
		t.Error("expected hearted=false for empty userID")
	}
}

func TestBulkCount(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT agent_name, COUNT").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name", "count"}).
			AddRow("agent-a", 10).
			AddRow("agent-b", 3))

	counts, err := s.BulkCount(ctx, "acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts["agent-a"] != 10 {
		t.Errorf("expected agent-a=10, got %d", counts["agent-a"])
	}
	if counts["agent-b"] != 3 {
		t.Errorf("expected agent-b=3, got %d", counts["agent-b"])
	}
}

func TestBulkIsHearted(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT agent_name FROM agent_hearts").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"agent_name"}).
			AddRow("agent-a").
			AddRow("agent-c"))

	hearted, err := s.BulkIsHearted(ctx, "acct-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hearted["agent-a"] {
		t.Error("expected agent-a hearted")
	}
	if hearted["agent-b"] {
		t.Error("expected agent-b not hearted")
	}
	if !hearted["agent-c"] {
		t.Error("expected agent-c hearted")
	}
}
