package riverqueue

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func backfillTestDB(t *testing.T) *sql.DB {
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
		t.Fatalf("ping: %v", err)
	}
	return db
}

func ensureBackfillTestAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`
		INSERT INTO accounts (name, type) VALUES ('test-backfill', 'personal')
		ON CONFLICT DO NOTHING RETURNING id
	`).Scan(&id); err != nil {
		if err := db.QueryRow(`SELECT id FROM accounts WHERE name = 'test-backfill'`).Scan(&id); err != nil {
			t.Fatalf("get test account: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM deployments WHERE account_id = $1`, id)
	})
	return id
}

type legacyVarFixture struct {
	name     string
	value    string
	ref      string
	secret   bool
	optional bool
	targets  []string
	nonce    []byte
}

func seedDeploymentWithLegacyVars(t *testing.T, db *sql.DB, accountID string, vars []legacyVarFixture) string {
	t.Helper()
	depID := deployid.New()
	if _, err := db.Exec(`
		INSERT INTO deployments (id, account_id, agent_name, build_id, namespace, deployment_spec_json, status)
		VALUES ($1, $2, 'backfill-agent', 'b1', 'ns-backfill', '{}', 'pending')
	`, depID, accountID); err != nil {
		t.Fatalf("insert deployment: %v", err)
	}
	for _, v := range vars {
		if _, err := db.Exec(`
			INSERT INTO deployment_variables (deployment_id, name, value, ref, secret, optional, targets, nonce)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, depID, v.name, v.value, v.ref, v.secret, v.optional, pq.Array(v.targets), v.nonce); err != nil {
			t.Fatalf("insert legacy var %s: %v", v.name, err)
		}
	}
	return depID
}

func newWorkerForTest(db *sql.DB) *BuildEnvBackfillWorker {
	return &BuildEnvBackfillWorker{
		db:  db,
		log: logger.New("error", "text"),
	}
}

// TestBackfillOne_RaceWithConcurrentWriterSurfacesUniqueViolation verifies that
// ON CONFLICT DO NOTHING makes the backfill INSERT safe against a concurrent
// dual-write that commits between the EXISTS check and the INSERT.
func TestBackfillOne_RaceWithConcurrentWriterSurfacesUniqueViolation(t *testing.T) {
	db := backfillTestDB(t)
	accountID := ensureBackfillTestAccount(t, db)

	depID := seedDeploymentWithLegacyVars(t, db, accountID, []legacyVarFixture{
		{name: "MY_VAR", value: "hello", secret: false, targets: []string{"agent"}},
	})

	// Open a transaction that holds an uncommitted row for the same key.
	// This simulates a concurrent dual-write that commits mid-backfill.
	holder, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin holder tx: %v", err)
	}
	defer holder.Rollback() //nolint:errcheck

	if _, err := holder.Exec(`
		INSERT INTO deployment_build_env
			(deployment_id, role, env_name, value_encrypted, nonce, is_secret, source)
		VALUES ($1, 'agent', 'MY_VAR', $2, NULL, false, 'user_var')
	`, depID, []byte("hello")); err != nil {
		t.Fatalf("seed concurrent row: %v", err)
	}

	w := newWorkerForTest(db)

	type result struct {
		written bool
		err     error
	}
	resultCh := make(chan result, 1)
	go func() {
		written, err := w.backfillOne(context.Background(), depID, "{}")
		resultCh <- result{written: written, err: err}
	}()

	time.Sleep(150 * time.Millisecond)

	if err := holder.Commit(); err != nil {
		t.Fatalf("commit holder: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Errorf("backfillOne should tolerate concurrent writes (ON CONFLICT DO NOTHING), got error: %v", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backfillOne hung waiting for the holder transaction")
	}
}

// TestWork_PerDeploymentFailureSurfaces verifies that Work returns a non-nil
// error when any per-deployment backfill fails, so River retries the job instead
// of marking it done.
func TestWork_PerDeploymentFailureSurfaces(t *testing.T) {
	db := backfillTestDB(t)
	accountID := ensureBackfillTestAccount(t, db)

	_ = seedDeploymentWithLegacyVars(t, db, accountID, []legacyVarFixture{
		{name: "OK_VAR", value: "ok", secret: false, targets: []string{"agent"}},
	})

	_ = seedDeploymentWithLegacyVars(t, db, accountID, []legacyVarFixture{
		{
			name:    "BAD_SECRET",
			value:   "this-is-not-base-64!!!",
			secret:  true,
			nonce:   []byte{0x01, 0x02, 0x03},
			targets: []string{"agent"},
		},
	})

	w := newWorkerForTest(db)
	if err := w.Work(context.Background(), nil); err == nil {
		t.Errorf("Work must return non-nil when per-deployment backfills fail (so River retries); got nil")
	}
}
