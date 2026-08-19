package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/astropods/astro/apps/astro-server/internal/workclassifier"
)

// TestLocalSeed drives the spend roll-up and the classification pass for one
// account, without starting the worker pool.
//
// The dev database is shared, and running the server with SERVER_MODE=all makes
// a laptop pick up every account's jobs and roll them up against whatever
// Langfuse that laptop is configured for. This stays scoped to one account.
//
//	LOCAL_SEED_ACCOUNT=chris-local LOCAL_SEED_DAYS=30 \
//	  go test ./handlers/ -run TestLocalSeed -v -timeout 30m
func TestLocalSeed(t *testing.T) {
	accountName := os.Getenv("LOCAL_SEED_ACCOUNT")
	if accountName == "" {
		t.Skip("set LOCAL_SEED_ACCOUNT to run")
	}
	days := 30
	if n, err := strconv.Atoi(os.Getenv("LOCAL_SEED_DAYS")); err == nil && n > 0 {
		days = n
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck

	log := logger.New("info", "text")
	accountStore := account.NewAccountStore(db)
	acct, err := accountStore.GetByName(accountName)
	if err != nil || acct == nil {
		t.Fatalf("account %q: %v", accountName, err)
	}

	rollup := &InsightsRollupProducer{
		Log:           log,
		Cfg:           cfg,
		AccountStore:  accountStore,
		LangfuseStore: langfuse.NewStore(db),
		SlackStore:    slackidentity.NewStore(db),
		MemberEmails:  memberemails.NewStore(db),
		Rollups:       insightsrollup.NewStore(db),
	}
	classifier := &ClassificationProducer{
		Log:           log,
		Cfg:           cfg,
		LangfuseStore: langfuse.NewStore(db),
		MemberEmails:  memberemails.NewStore(db),
		Classifier: workclassifier.NewClient(
			cfg.Deployment.FoundryInferenceURL,
			cfg.Deployment.WorkClassifierVersion,
		),
		Classifications: classification.NewStore(db),
	}

	ctx := context.Background()
	lastComplete := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	for i := days - 1; i >= 0; i-- {
		day := lastComplete.AddDate(0, 0, -i)
		if err := rollup.RollUpDay(ctx, acct.ID, day); err != nil {
			t.Logf("roll-up %s: %v", day.Format(time.DateOnly), err)
			continue
		}
		t.Logf("rolled up %s", day.Format(time.DateOnly))
	}

	// One pass advances by at most a tick's budget, so loop until the cursors
	// stop moving.
	for pass := 0; pass < 20; pass++ {
		before, _ := classifier.Classifications.GetState(ctx, acct.ID, classification.SourceClaudeCode)
		if err := classifier.ClassifyAccount(ctx, acct.ID); err != nil {
			t.Fatalf("classify pass %d: %v", pass, err)
		}
		after, _ := classifier.Classifications.GetState(ctx, acct.ID, classification.SourceClaudeCode)
		t.Logf("pass %d: through=%v from=%v complete=%v",
			pass, fmtDay(after.ClassifiedThrough), fmtDay(after.BackfilledFrom), after.BackfillComplete)
		if sameDay(before.ClassifiedThrough, after.ClassifiedThrough) &&
			sameDay(before.BackfilledFrom, after.BackfilledFrom) &&
			before.BackfillComplete == after.BackfillComplete {
			break
		}
	}
}

// TestLocalSourcePayload prints what the source detail endpoint would return
// for one account, straight from stored aggregates — no auth, no HTTP.
//
//	LOCAL_SEED_ACCOUNT=chris-local go test ./handlers/ -run TestLocalSourcePayload -v
func TestLocalSourcePayload(t *testing.T) {
	accountName := os.Getenv("LOCAL_SEED_ACCOUNT")
	if accountName == "" {
		t.Skip("set LOCAL_SEED_ACCOUNT to run")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck

	acct, err := account.NewAccountStore(db).GetByName(accountName)
	if err != nil || acct == nil {
		t.Fatalf("account %q: %v", accountName, err)
	}
	ad, ok := devtoolAdapterByKey(classification.SourceClaudeCode)
	if !ok {
		t.Fatal("claude-code adapter missing")
	}
	resp, err := computeInsightsSource(context.Background(),
		classification.NewStore(db), acct.ID, ad, sourceViewer{}, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	out, _ := json.MarshalIndent(resp, "", "  ")
	if dest := os.Getenv("LOCAL_SEED_OUT"); dest != "" {
		if err := os.WriteFile(dest, out, 0o600); err != nil {
			t.Fatalf("write %s: %v", dest, err)
		}
		t.Logf("wrote %s", dest)
		return
	}
	t.Logf("\n%s", out)
}

func fmtDay(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.DateOnly)
}

func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.UTC().Truncate(24 * time.Hour).Equal(b.UTC().Truncate(24 * time.Hour))
}
