package appstore

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	return db
}

func testAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		WITH acct AS (
			INSERT INTO accounts (name, type, owner_user_id) VALUES ('test-apps-account', 'organization', 'test-apps-owner')
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		), member AS (
			INSERT INTO account_members (account_id, user_id) SELECT id, 'test-apps-owner' FROM acct
			ON CONFLICT DO NOTHING
		)
		SELECT id FROM acct
	`).Scan(&id)
	if err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	return id
}

func newApp(t *testing.T, s *Store, accountID string, scopes []string) *App {
	t.Helper()
	suffix := deployid.New()
	app, err := s.Create(context.Background(), CreateParams{
		AccountID:           accountID,
		Name:                "app-" + suffix,
		WorkOSApplicationID: "workos_" + suffix,
		ClientID:            "client_" + suffix,
		Scopes:              scopes,
		CreatedBy:           "tester",
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() { _, _ = s.db.Exec(`DELETE FROM account_apps WHERE id = $1`, app.ID) })
	return app
}

func TestCreateWithNoScopes(t *testing.T) {
	s := NewStore(testDB(t))
	accountID := testAccount(t, s.db)

	app := newApp(t, s, accountID, nil)
	if app.Scopes == nil {
		app.Scopes = []string{}
	}
	if len(app.Scopes) != 0 {
		t.Fatalf("scopes = %+v, want empty", app.Scopes)
	}
}

func TestCreateWithScopesRoundTrips(t *testing.T) {
	s := NewStore(testDB(t))
	accountID := testAccount(t, s.db)

	created := newApp(t, s, accountID, []string{"audiences:read", "audiences:manage"})
	found, err := s.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if len(found.Scopes) != 2 || found.Scopes[1] != "audiences:manage" {
		t.Fatalf("scopes = %+v", found.Scopes)
	}
}

func TestGetByClientID(t *testing.T) {
	s := NewStore(testDB(t))
	accountID := testAccount(t, s.db)
	created := newApp(t, s, accountID, nil)

	found, err := s.GetByClientID(context.Background(), created.ClientID)
	if err != nil {
		t.Fatalf("get by client id: %v", err)
	}
	if found == nil || found.ID != created.ID {
		t.Fatalf("got %+v, want %s", found, created.ID)
	}

	missing, err := s.GetByClientID(context.Background(), "client_"+deployid.New())
	if err != nil {
		t.Fatalf("get unknown client id: %v", err)
	}
	if missing != nil {
		t.Fatal("an unknown client must resolve to nothing, so its token is denied")
	}
}

func TestCreateRejectsDuplicateName(t *testing.T) {
	s := NewStore(testDB(t))
	accountID := testAccount(t, s.db)
	first := newApp(t, s, accountID, nil)

	suffix := deployid.New()
	_, err := s.Create(context.Background(), CreateParams{
		AccountID:           accountID,
		Name:                first.Name,
		WorkOSApplicationID: "workos_" + suffix,
		ClientID:            "client_" + suffix,
	})
	if err != ErrNameTaken {
		t.Fatalf("err = %v, want ErrNameTaken", err)
	}
}

func TestDeleteRemovesTheRow(t *testing.T) {
	s := NewStore(testDB(t))
	accountID := testAccount(t, s.db)
	app := newApp(t, s, accountID, nil)

	if err := s.Delete(context.Background(), app.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	found, err := s.GetByClientID(context.Background(), app.ClientID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if found != nil {
		t.Fatal("a deleted app must stop resolving, which is what revokes its tokens")
	}
}

func TestListByAccount(t *testing.T) {
	s := NewStore(testDB(t))
	accountID := testAccount(t, s.db)
	created := newApp(t, s, accountID, nil)

	apps, err := s.ListByAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, a := range apps {
		if a.ID == created.ID {
			return
		}
	}
	t.Fatalf("created app missing from the account list: %+v", apps)
}

func TestValidateName(t *testing.T) {
	if err := ValidateName("lumos-connector"); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	for _, name := range []string{"", "two\nlines", string(make([]byte, 101))} {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want failure", name)
		}
	}
}
