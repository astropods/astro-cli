package eventstream

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreSinceResolvesRecipientsAtReadTime(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`(?s)SELECT e\.id.+FROM agent_events e`).
		WithArgs("acct-1", int64(5), replayLimit+1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "type", "build_id", "status"}).
			AddRow(int64(6), "reviewer", "agent.build", "b1", "registered").
			AddRow(int64(7), "planner", "agent.build", "b2", "building"))

	got, more, err := NewStore(db).Since(context.Background(), "acct-1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if more {
		t.Fatal("more = true, want false")
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Rows carry the publisher's account; the reader's is what the client needs.
	if got[0].ID != "6" || got[0].AccountID != "acct-1" || got[0].Agent != "reviewer" {
		t.Fatalf("first event = %+v", got[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreSinceReportsTruncationWithoutOverreturning(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	// One past the cap: the extra row is the signal, and must not be returned,
	// or the caller would advance its cursor past an event it never wrote out.
	rows := sqlmock.NewRows([]string{"id", "agent_name", "type", "build_id", "status"})
	for i := 1; i <= replayLimit+1; i++ {
		rows.AddRow(int64(i), "a", "agent.build", "b", "registered")
	}
	mock.ExpectQuery(`(?s)SELECT e\.id.+FROM agent_events e`).WillReturnRows(rows)

	got, more, err := NewStore(db).Since(context.Background(), "acct-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("more = false, want true")
	}
	if len(got) != replayLimit {
		t.Fatalf("len = %d, want %d", len(got), replayLimit)
	}
}

func TestStoreMaxIDTreatsAnEmptyTableAsZero(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("SELECT max\\(id\\) FROM agent_events").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil))

	id, err := NewStore(db).MaxID(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Fatalf("MaxID = %d, want 0", id)
	}
}

func TestStoreRecordReturnsTheAssignedID(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	mock.ExpectQuery("(?s)INSERT INTO agent_events").
		WithArgs("acct-1", "reviewer", "agent.build", "b1", "registered").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))

	e, err := NewStore(db).Record(context.Background(), nil, "acct-1", "reviewer", "agent.build", "b1", "registered")
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "42" {
		t.Fatalf("ID = %q, want 42", e.ID)
	}
}
