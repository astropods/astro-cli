package riverqueue

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type fakeOrgProvisionQueue struct{ enqueued []string }

func (f *fakeOrgProvisionQueue) InsertAccountOrgProvision(_ context.Context, accountID string) error {
	f.enqueued = append(f.enqueued, accountID)
	return nil
}

func TestAccountOrgProvisionSweep_EnqueuesUnlinkedAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`LEFT JOIN account_organizations ao`).
		WithArgs(orgProvisionSweepLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "created_at", "updated_at"}).
			AddRow("acct-1", "saswat", "personal", time.Now(), time.Now()).
			AddRow("acct-2", "acme", "organization", time.Now(), time.Now()))

	q := &fakeOrgProvisionQueue{}
	w := &AccountOrgProvisionSweepWorker{
		accounts: account.NewAccountStore(db),
		queue:    q,
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[AccountOrgProvisionSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(q.enqueued) != 2 || q.enqueued[0] != "acct-1" || q.enqueued[1] != "acct-2" {
		t.Errorf("enqueued = %v, want [acct-1 acct-2]", q.enqueued)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAccountOrgProvisionSweep_NoQueueIsInert(t *testing.T) {
	w := &AccountOrgProvisionSweepWorker{log: logger.New("error", "json")}
	if err := w.Work(context.Background(), &river.Job[AccountOrgProvisionSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}
