package accountlifecycle

import (
	"context"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func newPurger(t *testing.T) (*Purger, sqlmock.Sqlmock, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	db, dbMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("account sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	deployDB, deployMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("deploy sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = deployDB.Close() })

	requeued := &[]string{}
	return &Purger{
		Log:         logger.New("error", "json"),
		DB:          db,
		Deployments: deploymentstore.NewStore(deployDB),
		Undeploy: func(_ context.Context, id string) error {
			*requeued = append(*requeued, id)
			return nil
		},
	}, dbMock, deployMock, requeued
}

// Every key-revocation step reads a store the caller never passes, so a
// provisioner arriving without its store would skip the revoke and leave a
// working credential behind.
func TestNewPurger_DerivesEachProvisionerStore(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bare := NewPurger(PurgerDeps{Log: logger.New("error", "json"), DB: db})
	if bare.LangfuseStore != nil || bare.Keys != nil || bare.DevKeys != nil || bare.JudgeKeys != nil {
		t.Fatal("unconfigured backends should leave their stores nil")
	}

	full := NewPurger(PurgerDeps{
		Log:       logger.New("error", "json"),
		DB:        db,
		Langfuse:  &langfuse.Provisioner{},
		AIGateway: aigateway.NewProvisioner(nil, nil, nil),
	})
	if full.LangfuseStore == nil {
		t.Error("Langfuse provisioner set, LangfuseStore nil")
	}
	if full.Keys == nil || full.DevKeys == nil || full.JudgeKeys == nil {
		t.Error("AI Gateway provisioner set, key stores nil")
	}
}

// An account whose deployments are still up must survive the purge: hard-deleting
// the row drops the record of what is running, and nothing else would tear it down.
func TestPurge_RefusesWhileDeploymentsRemain(t *testing.T) {
	p, dbMock, deployMock, requeued := newPurger(t)

	now := time.Now()
	rev := 1
	deployMock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(deploymentColumns).AddRow(
			"dep-1", "acct-1", nil, "agent", "b1", "ns-1", "Agent", `{}`, nil, nil, nil,
			"active", nil, nil, now, &rev, now, nil, nil, nil,
		))

	err := p.Purge(context.Background(), "acct-1")
	if !errors.Is(err, ErrTeardownPending) {
		t.Fatalf("err = %v, want ErrTeardownPending", err)
	}
	if !strings.Contains(err.Error(), "1 deployment") {
		t.Errorf("error should name the blocker, got %v", err)
	}
	// The stuck deployment is re-queued so the next attempt can succeed.
	if len(*requeued) != 1 || (*requeued)[0] != "dep-1" {
		t.Errorf("requeued = %v, want [dep-1]", *requeued)
	}
	// No DELETE was expected, so an attempt to hard-delete fails here.
	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account mock: %v", err)
	}
}

// A gateway outage must not block the purge forever: the key rows go with the
// account either way, and the sweep would otherwise retry this account nightly.
func TestPurge_HardDeletesAfterAnUpstreamKeyRevokeFails(t *testing.T) {
	p, dbMock, deployMock, _ := newPurger(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(upstream.Close)

	judgeDB, judgeMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("judge sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = judgeDB.Close() })

	p.AIGateway = aigateway.NewProvisioner(aigateway.NewClient(upstream.URL, upstream.URL, ""), nil, nil)
	p.JudgeKeys = aigateway.NewJudgeStore(judgeDB)

	deployMock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(deploymentColumns))
	judgeMock.ExpectQuery("SELECT key_id FROM account_llm_judge_keys").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"key_id"}).AddRow("vk-judge"))
	dbMock.ExpectExec("DELETE FROM accounts WHERE id").
		WithArgs("acct-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := p.Purge(context.Background(), "acct-1"); err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account mock: %v", err)
	}
	if err := judgeMock.ExpectationsWereMet(); err != nil {
		t.Errorf("judge mock: %v", err)
	}
}

// cutoffNearDaysAgo matches a timestamp argument roughly n days in the past, so
// the test states the retention window without pinning the exact clock reading.
type cutoffNearDaysAgo int

func (c cutoffNearDaysAgo) Match(v driver.Value) bool {
	ts, ok := v.(time.Time)
	if !ok {
		return false
	}
	drift := time.Since(ts) - time.Duration(c)*24*time.Hour
	return drift > -time.Hour && drift < time.Hour
}

// The sweep must only see accounts past the retention window; a zero cutoff
// would hand it every soft-deleted account, including today's.
func TestOverdue_CutsOffAtTheRetentionWindow(t *testing.T) {
	p, dbMock, _, _ := newPurger(t)

	dbMock.ExpectQuery("SELECT id FROM accounts WHERE deleted_at IS NOT NULL").
		WithArgs(cutoffNearDaysAgo(RetentionDays)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acct-1"))

	ids, err := p.Overdue(context.Background())
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(ids) != 1 || ids[0] != "acct-1" {
		t.Fatalf("ids = %v, want [acct-1]", ids)
	}
	if err := dbMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account mock: %v", err)
	}
}
