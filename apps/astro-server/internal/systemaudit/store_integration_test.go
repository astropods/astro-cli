//go:build integration

package systemaudit

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestChecksAreValidSQL(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, c := range Checks() {
		rows, err := db.QueryContext(ctx, c.Query)
		if err != nil {
			t.Errorf("check %s: %v", c.Name, err)
			continue
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Errorf("check %s columns: %v", c.Name, err)
		}
		if len(cols) != 3 {
			t.Errorf("check %s returned %d columns, want subject_id, subject_label, detail", c.Name, len(cols))
		}
		_ = rows.Close()
	}
}

func TestRunRecordsResolvesAndReopens(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := NewStore(db)

	if _, err := db.ExecContext(ctx, `DELETE FROM system_audit_findings WHERE check_name = 'test.fixture'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM system_audit_findings WHERE check_name = 'test.fixture'`)
	})

	fixture := Check{
		Name:     "test.fixture",
		Severity: SeverityWarning,
		Title:    "Fixture",
		Query:    `SELECT 'subject-1', 'Subject One', jsonb_build_object('n', 1)`,
	}

	open, resolved, err := store.runCheck(ctx, fixture)
	if err != nil || open != 1 || resolved != 0 {
		t.Fatalf("first run = (%d, %d, %v), want (1, 0, nil)", open, resolved, err)
	}

	findings, err := store.List(ctx, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	first := findingFor(findings, "test.fixture")
	if first == nil {
		t.Fatal("finding not recorded")
	}
	if err := store.Acknowledge(ctx, "test.fixture", "subject-1"); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	empty := fixture
	empty.Query = `SELECT 'subject-1', 'Subject One', '{}'::jsonb WHERE false`
	if _, resolved, err = store.runCheck(ctx, empty); err != nil || resolved != 1 {
		t.Fatalf("second run resolved = (%d, %v), want (1, nil)", resolved, err)
	}
	if got, _ := store.List(ctx, false); findingFor(got, "test.fixture") != nil {
		t.Fatal("resolved finding still listed as open")
	}

	time.Sleep(10 * time.Millisecond)
	if _, _, err := store.runCheck(ctx, fixture); err != nil {
		t.Fatalf("third run: %v", err)
	}
	reopened := findingFor(mustList(t, store, ctx), "test.fixture")
	if reopened == nil {
		t.Fatal("finding did not reopen")
	}
	if reopened.AcknowledgedAt != nil {
		t.Error("acknowledgement survived a resolve/reopen cycle; the second occurrence is a new decision")
	}
	if !reopened.FirstSeenAt.After(first.FirstSeenAt) {
		t.Error("first_seen_at was not reset when the finding came back")
	}
}

func checkNamed(t *testing.T, name string) Check {
	t.Helper()
	for _, c := range Checks() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q", name)
	return Check{}
}

func mustList(t *testing.T, s *Store, ctx context.Context) []Finding {
	t.Helper()
	got, err := s.List(ctx, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return got
}

func findingFor(findings []Finding, check string) *Finding {
	for i := range findings {
		if findings[i].CheckName == check {
			return &findings[i]
		}
	}
	return nil
}
