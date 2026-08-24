package accountlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// stubBilling answers DeleteCustomer and nothing else; the delete sequence
// touches no other provider method, and a nil embedded interface panics loudly
// if that ever stops being true.
type stubBilling struct {
	billing.BillingProvider
	err   error
	calls []string
}

func (s *stubBilling) DeleteCustomer(_ context.Context, customerID string) error {
	s.calls = append(s.calls, customerID)
	return s.err
}

var deploymentColumns = []string{
	"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
	"display_name", "deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
	"status", "error_message", "error_details", "status_changed_at", "current_revision",
	"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
}

func newDeleter(t *testing.T, bill billing.BillingProvider) (*Deleter, sqlmock.Sqlmock, sqlmock.Sqlmock, *[]string) {
	t.Helper()
	accountDB, accountMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("account sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = accountDB.Close() })
	deployDB, deployMock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("deploy sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = deployDB.Close() })

	undeployed := &[]string{}
	return &Deleter{
		Log:         logger.New("error", "json"),
		Accounts:    account.NewAccountStore(accountDB),
		Deployments: deploymentstore.NewStore(deployDB),
		Undeploy: func(_ context.Context, dep *deploymentstore.Deployment) error {
			*undeployed = append(*undeployed, dep.ID)
			return nil
		},
		Billing:        bill,
		BillingBackend: "metronome",
	}, accountMock, deployMock, undeployed
}

// A soft-deleted account still attached to a live billing customer keeps
// charging someone nobody is watching, so a failed archive has to leave the
// account intact for a retry.
func TestDelete_LeavesTheAccountAliveWhenBillingArchiveFails(t *testing.T) {
	bill := &stubBilling{err: errors.New("metronome unavailable")}
	d, accountMock, _, undeployed := newDeleter(t, bill)

	accountMock.ExpectQuery("SELECT metronome_customer_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	_, err := d.Delete(context.Background(), &account.Account{ID: "acct-1", Name: "defunct"})
	if err == nil {
		t.Fatal("expected the delete to abort")
	}
	if len(bill.calls) != 1 {
		t.Errorf("archive calls = %v, want [cust-1]", bill.calls)
	}
	if len(*undeployed) != 0 {
		t.Errorf("nothing should have been torn down: %v", *undeployed)
	}
	// The soft-delete UPDATE was never expected, so an attempt fails here.
	if err := accountMock.ExpectationsWereMet(); err != nil {
		t.Errorf("account mock: %v", err)
	}
}

// One deployment that cannot be queued must not strand the rest: the purge
// worker retries the stragglers, but only if the account is already deleted.
func TestDelete_CountsOnlyTheDeploymentsItQueued(t *testing.T) {
	d, accountMock, deployMock, undeployed := newDeleter(t, nil)
	d.Undeploy = func(_ context.Context, dep *deploymentstore.Deployment) error {
		if dep.ID == "dep-1" {
			return errors.New("queue unavailable")
		}
		*undeployed = append(*undeployed, dep.ID)
		return nil
	}

	now := time.Now()
	rev := 1
	accountMock.ExpectExec("UPDATE accounts SET deleted_at").
		WillReturnResult(sqlmock.NewResult(0, 1))
	deployMock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(deploymentColumns).
			AddRow("dep-1", "acct-1", nil, "a", "b1", "ns-1", "A", `{}`, nil, nil, nil, "active", nil, nil, now, &rev, now, nil, nil, nil).
			AddRow("dep-2", "acct-1", nil, "b", "b2", "ns-2", "B", `{}`, nil, nil, nil, "active", nil, nil, now, &rev, now, nil, nil, nil))

	result, err := d.Delete(context.Background(), &account.Account{ID: "acct-1", Name: "defunct"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if result.DeploymentsUndeploying != 1 {
		t.Errorf("DeploymentsUndeploying = %d, want 1", result.DeploymentsUndeploying)
	}
	if len(*undeployed) != 1 || (*undeployed)[0] != "dep-2" {
		t.Errorf("undeployed = %v, want [dep-2]", *undeployed)
	}
}

func TestDelete_ReportsAnAlreadyDeletedAccount(t *testing.T) {
	d, accountMock, _, _ := newDeleter(t, nil)

	accountMock.ExpectExec("UPDATE accounts SET deleted_at").
		WillReturnResult(sqlmock.NewResult(0, 0))

	_, err := d.Delete(context.Background(), &account.Account{ID: "acct-1", Name: "defunct"})
	if !errors.Is(err, account.ErrAlreadyDeleted) {
		t.Fatalf("err = %v, want ErrAlreadyDeleted", err)
	}
}
