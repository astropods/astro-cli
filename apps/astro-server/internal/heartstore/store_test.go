package heartstore

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestToggle_AddHeart(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("WITH toggled AS").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"hearted", "count"}).AddRow(true, 5))

	hearted, count, err := s.Toggle(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hearted {
		t.Error("expected hearted=true")
	}
	if count != 5 {
		t.Errorf("expected count=5, got %d", count)
	}
}

func TestToggle_RemoveHeart(t *testing.T) {
	db, mock, _ := sqlmock.New()
	s := New(db)
	ctx := context.Background()

	mock.ExpectQuery("WITH toggled AS").
		WithArgs("acct-1", "my-agent", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"hearted", "count"}).AddRow(false, 4))

	hearted, count, err := s.Toggle(ctx, "acct-1", "my-agent", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hearted {
		t.Error("expected hearted=false")
	}
	if count != 4 {
		t.Errorf("expected count=4, got %d", count)
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
