package riverqueue

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/identitygen"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func newBlueprintBackfillWorker(t *testing.T) (*BlueprintAvatarBackfillWorker, sqlmock.Sqlmock, *avatar.Store, string) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	store := avatar.NewStore(avatar.NewLocalBackend(dir), "")

	w := &BlueprintAvatarBackfillWorker{
		avatarStore: store,
		db:          db,
		log:         logger.New("error", "json"),
	}
	return w, mock, store, dir
}

const blueprintQuery = `
			SELECT a.account_id::text, a.name, acc.name
			FROM agents a
			JOIN accounts acc ON acc.id = a.account_id
			WHERE a.archived_at IS NULL
			  AND ($1 = '' OR (a.account_id, a.name) > ($1::uuid, $2))
			ORDER BY a.account_id, a.name
			LIMIT $3
		`

const deploymentQuery = `
			SELECT d.id, d.agent_name, owner.name
			FROM deployments d
			JOIN accounts owner ON owner.id = COALESCE(d.source_account_id, d.account_id)
			WHERE ($1 = '' OR d.id > $1)
			ORDER BY d.id
			LIMIT $2
		`

// UUIDs used as account_id in the fixture rows. The query cursor uses
// (account_id, name) tuple ordering so we need stable, ordered values.
const (
	acctIDA = "11111111-1111-1111-1111-111111111111"
	acctIDB = "22222222-2222-2222-2222-222222222222"
)

func TestBlueprintAvatarBackfill_GeneratesMissingAvatars(t *testing.T) {
	w, mock, store, _ := newBlueprintBackfillWorker(t)
	ctx := context.Background()

	// First batch returns two blueprints; second batch returns zero rows to
	// terminate the loop.
	mock.ExpectQuery(blueprintQuery).
		WithArgs("", "", 100).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "account_name"}).
			AddRow(acctIDA, "bot-a", "acme").
			AddRow(acctIDB, "bot-b", "widgets"))
	mock.ExpectQuery(blueprintQuery).
		WithArgs(acctIDB, "bot-b", 100).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "account_name"}))

	// No deployments.
	mock.ExpectQuery(deploymentQuery).
		WithArgs("", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "account_name"}))

	if err := w.Work(ctx, &river.Job[BlueprintAvatarBackfillArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	// Both blueprints' avatars must now exist.
	for _, bp := range []struct{ acct, name string }{
		{"acme", "bot-a"},
		{"widgets", "bot-b"},
	} {
		exists, err := store.AgentAvatarExists(ctx, bp.acct, bp.name)
		if err != nil {
			t.Fatalf("exists %s/%s: %v", bp.acct, bp.name, err)
		}
		if !exists {
			t.Fatalf("avatar for %s/%s should exist", bp.acct, bp.name)
		}
	}
}

func TestBlueprintAvatarBackfill_SkipsExistingAvatars(t *testing.T) {
	w, mock, store, dir := newBlueprintBackfillWorker(t)
	ctx := context.Background()

	// Pre-seed an avatar for acme/bot-a so the worker should skip it.
	const preseedContent = "existing-content"
	if err := store.WriteAgentAvatarJPEG(ctx, "acme", "bot-a", []byte(preseedContent)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	mock.ExpectQuery(blueprintQuery).
		WithArgs("", "", 100).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "account_name"}).
			AddRow(acctIDA, "bot-a", "acme"))
	mock.ExpectQuery(blueprintQuery).
		WithArgs(acctIDA, "bot-a", 100).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "account_name"}))
	mock.ExpectQuery(deploymentQuery).
		WithArgs("", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "account_name"}))

	if err := w.Work(ctx, &river.Job[BlueprintAvatarBackfillArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	// The pre-seeded content should not have been overwritten.
	data, err := os.ReadFile(filepath.Join(dir, "avatars", "agents", "acme", "bot-a.jpg"))
	if err != nil {
		t.Fatalf("read pre-seeded: %v", err)
	}
	if string(data) != preseedContent {
		t.Fatalf("existing avatar was overwritten: got %d bytes", len(data))
	}
}

func TestBlueprintAvatarBackfill_CopiesBlueprintAvatarToDeployment(t *testing.T) {
	w, mock, store, _ := newBlueprintBackfillWorker(t)
	ctx := context.Background()

	// Blueprint pass: returns acme/bot-a, then stops.
	mock.ExpectQuery(blueprintQuery).
		WithArgs("", "", 100).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "account_name"}).
			AddRow(acctIDA, "bot-a", "acme"))
	mock.ExpectQuery(blueprintQuery).
		WithArgs(acctIDA, "bot-a", 100).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "account_name"}))

	// Deployment pass: deploy-1 for acme/bot-a.
	mock.ExpectQuery(deploymentQuery).
		WithArgs("", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "account_name"}).
			AddRow("deploy-1", "bot-a", "acme"))
	mock.ExpectQuery(deploymentQuery).
		WithArgs("deploy-1", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_name", "account_name"}))

	if err := w.Work(ctx, &river.Job[BlueprintAvatarBackfillArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	exists, err := store.DeploymentAvatarExists(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("deployment exists: %v", err)
	}
	if !exists {
		t.Fatal("deployment avatar should exist after backfill")
	}
}

// expectDeploymentBatch queues one batch holding a single deployment row plus
// the empty batch that ends the cursor loop.
func expectDeploymentBatch(mock sqlmock.Sqlmock, id, agentName, ownerName string) {
	cols := []string{"id", "agent_name", "owner_name"}
	mock.ExpectQuery(deploymentQuery).
		WithArgs("", 100).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(id, agentName, ownerName))
	mock.ExpectQuery(deploymentQuery).
		WithArgs(id, 100).
		WillReturnRows(sqlmock.NewRows(cols))
}

func TestBlueprintAvatarBackfill_CopiesFromBlueprintOwnerOnCrossAccountDeploy(t *testing.T) {
	w, mock, store, _ := newBlueprintBackfillWorker(t)
	ctx := context.Background()

	// acme owns the blueprint; widgets deployed it. The query resolves the
	// owner, so the avatar must come from acme's key, not the deployer's.
	const blueprintAvatar = "acme-bot-a-bytes"
	if err := store.WriteAgentAvatarJPEG(ctx, "acme", "bot-a", []byte(blueprintAvatar)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}
	expectDeploymentBatch(mock, "deploy-x", "bot-a", "acme")

	processed, _, failed := w.backfillDeployments(ctx)
	if processed != 1 || failed != 0 {
		t.Fatalf("backfillDeployments() = processed %d, failed %d; want 1, 0", processed, failed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}

	got, err := store.ReadDeploymentAvatar(ctx, "deploy-x")
	if err != nil {
		t.Fatalf("read deployment avatar: %v", err)
	}
	if string(got) != blueprintAvatar {
		t.Fatalf("deployment avatar = %q, want the blueprint owner's bytes", got)
	}
}

func TestBlueprintAvatarBackfill_GeneratesWhenBlueprintAvatarGone(t *testing.T) {
	w, mock, store, _ := newBlueprintBackfillWorker(t)
	ctx := context.Background()

	// No blueprint avatar exists, standing in for an archived or deleted
	// blueprint whose deployments are still around.
	expectDeploymentBatch(mock, "deploy-y", "retired", "acme")

	processed, _, failed := w.backfillDeployments(ctx)
	if processed != 1 || failed != 0 {
		t.Fatalf("backfillDeployments() = processed %d, failed %d; want 1, 0", processed, failed)
	}

	got, err := store.ReadDeploymentAvatar(ctx, "deploy-y")
	if err != nil {
		t.Fatalf("read deployment avatar: %v", err)
	}
	want, err := identitygen.GenerateIdentityJPEG(identitygen.IdentityOptions{Seed: "acme/retired"})
	if err != nil {
		t.Fatalf("generate reference image: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("deployment avatar (%d bytes) is not the blueprint seed image (%d bytes)", len(got), len(want))
	}
}

func TestBlueprintAvatarBackfill_SkipsDeploymentThatAlreadyHasAvatar(t *testing.T) {
	w, mock, store, _ := newBlueprintBackfillWorker(t)
	ctx := context.Background()

	// A filled deployment must not be touched again, so a run after the
	// generate above stays quiet instead of rewriting every 24h.
	const existing = "already-there"
	if err := store.WriteDeploymentAvatarJPEG(ctx, "deploy-z", []byte(existing)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}
	expectDeploymentBatch(mock, "deploy-z", "retired", "acme")

	processed, skipped, failed := w.backfillDeployments(ctx)
	if processed != 0 || skipped != 1 || failed != 0 {
		t.Fatalf("backfillDeployments() = processed %d, skipped %d, failed %d; want 0, 1, 0", processed, skipped, failed)
	}

	got, err := store.ReadDeploymentAvatar(ctx, "deploy-z")
	if err != nil {
		t.Fatalf("read deployment avatar: %v", err)
	}
	if string(got) != existing {
		t.Fatalf("deployment avatar was overwritten: got %d bytes", len(got))
	}
}
