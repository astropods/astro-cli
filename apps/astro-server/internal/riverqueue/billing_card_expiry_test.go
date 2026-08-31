package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

// zeroUUID is the keyset seed ListCarded substitutes for an empty cursor.
const zeroUUID = "00000000-0000-0000-0000-000000000000"

type fakeCardExpiryAccounts struct {
	fakeAccountNamer
	customers map[string]string
	err       error
}

func (f fakeCardExpiryAccounts) GetStripeCustomerID(accountID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.customers[accountID], nil
}

type fakeCardVault struct {
	cards map[string]*payment.Card
	err   error
}

func (f fakeCardVault) DefaultCard(_ context.Context, customerID string) (*payment.Card, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cards[customerID], nil
}

func cardedRow(status billing.Status, creditsExhausted bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"status", "reason", "dunning_since", "alert_active",
		"force_suspended", "credits_exhausted", "has_payment_method", "pay_link",
		"usage_limit_active", "not_provisioned",
	}).AddRow(string(status), "", nil, false, false, creditsExhausted, false, nil, false, false)
}

func expectListCarded(mock sqlmock.Sqlmock, after string, accountIDs ...string) {
	rows := sqlmock.NewRows([]string{"account_id"})
	for _, id := range accountIDs {
		rows.AddRow(id)
	}
	mock.ExpectQuery(`FROM account_billing_status\s+WHERE has_payment_method`).
		WithArgs(after, cardExpiryPageSize).
		WillReturnRows(rows)
}

func TestCardExpirySweep_ExpiredCardSuspendsAnExhaustedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	expectListCarded(mock, zeroUUID, "acct_1")
	mock.ExpectExec("has_payment_method").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_1").
		WillReturnRows(cardedRow(billing.StatusActive, true))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &fakeDunningQueue{}
	w := &CardExpirySweepWorker{
		status: billing.NewStatusStore(db, 7),
		cards: fakeCardVault{cards: map[string]*payment.Card{
			"cus_1": {ExpMonth: 1, ExpYear: 2020},
		}},
		accounts: fakeCardExpiryAccounts{
			fakeAccountNamer: fakeAccountNamer{names: map[string]string{"acct_1": "acme"}},
			customers:        map[string]string{"acct_1": "cus_1"},
		},
		queue: q,
		log:   logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[CardExpirySweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expired card did not clear the payment method: %v", err)
	}
	if len(q.suspended) != 1 || q.suspended[0] != "acct_1" {
		t.Errorf("suspended = %v, want [acct_1]: the agents keep running on a card nothing can charge", q.suspended)
	}
	if len(q.budgets) != 1 {
		t.Errorf("gateway budget re-derived %d times, want 1: the ceiling stays at the carded default", len(q.budgets))
	}
	if len(q.notifiedNames) != 1 || q.notifiedNames[0] != "acme" {
		t.Errorf("notified names = %q, want [acme]", q.notifiedNames)
	}
}

func TestCardExpirySweep_ExpiredCardInsideCreditDoesNotSuspend(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	expectListCarded(mock, zeroUUID, "acct_1")
	mock.ExpectExec("has_payment_method").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_1").
		WillReturnRows(cardedRow(billing.StatusActive, false))
	mock.ExpectRollback()

	q := &fakeDunningQueue{}
	w := &CardExpirySweepWorker{
		status: billing.NewStatusStore(db, 7),
		cards: fakeCardVault{cards: map[string]*payment.Card{
			"cus_1": {ExpMonth: 1, ExpYear: 2020},
		}},
		accounts: fakeCardExpiryAccounts{customers: map[string]string{"acct_1": "cus_1"}},
		queue:    q,
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[CardExpirySweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(q.suspended) != 0 {
		t.Errorf("suspended = %v, want none: an account inside its signup credit owes nothing", q.suspended)
	}
	if len(q.notified) != 0 {
		t.Errorf("notified = %v, want none: nothing happened to the account", q.notified)
	}
	if len(q.budgets) != 1 {
		t.Errorf("gateway budget re-derived %d times, want 1", len(q.budgets))
	}
}

func TestCardExpirySweep_LeavesValidCardsAlone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	expectListCarded(mock, zeroUUID, "acct_1", "acct_2")

	q := &fakeDunningQueue{}
	next := time.Now().AddDate(1, 0, 0)
	w := &CardExpirySweepWorker{
		status: billing.NewStatusStore(db, 7),
		cards: fakeCardVault{cards: map[string]*payment.Card{
			"cus_1": {ExpMonth: int64(next.Month()), ExpYear: int64(next.Year())},
			"cus_2": nil,
		}},
		accounts: fakeCardExpiryAccounts{customers: map[string]string{"acct_1": "cus_1", "acct_2": "cus_2"}},
		queue:    q,
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[CardExpirySweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a valid card was written to: %v", err)
	}
	if len(q.suspended)+len(q.notified)+len(q.budgets) != 0 {
		t.Errorf("valid cards produced work: suspended=%v notified=%v budgets=%v", q.suspended, q.notified, q.budgets)
	}
}

func TestCardExpirySweep_OneUnreadableAccountDoesNotStopTheRest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	expectListCarded(mock, zeroUUID, "acct_bad", "acct_2")
	mock.ExpectExec("has_payment_method").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_2").
		WillReturnRows(cardedRow(billing.StatusActive, true))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &fakeDunningQueue{}
	w := &CardExpirySweepWorker{
		status: billing.NewStatusStore(db, 7),
		cards: fakeCardVault{cards: map[string]*payment.Card{
			"cus_2": {ExpMonth: 1, ExpYear: 2020},
		}},
		accounts: errorForAccount{
			fakeCardExpiryAccounts: fakeCardExpiryAccounts{customers: map[string]string{"acct_bad": "cus_bad", "acct_2": "cus_2"}},
			bad:                    "acct_bad",
		},
		queue: q,
		log:   logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[CardExpirySweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the sweep stopped early: %v", err)
	}
	if len(q.suspended) != 1 || q.suspended[0] != "acct_2" {
		t.Errorf("suspended = %v, want [acct_2]", q.suspended)
	}
}

type errorForAccount struct {
	fakeCardExpiryAccounts
	bad string
}

func (f errorForAccount) GetStripeCustomerID(accountID string) (string, error) {
	if accountID == f.bad {
		return "", errors.New("customer lookup down")
	}
	return f.fakeCardExpiryAccounts.GetStripeCustomerID(accountID)
}

func TestCardExpirySweep_WalksPastTheFirstPage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	full := make([]string, cardExpiryPageSize)
	cards := map[string]*payment.Card{}
	customers := map[string]string{}
	valid := time.Now().AddDate(1, 0, 0)
	for i := range full {
		id := fmt.Sprintf("acct_%03d", i)
		full[i] = id
		customers[id] = "cus_" + id
		cards["cus_"+id] = &payment.Card{ExpMonth: int64(valid.Month()), ExpYear: int64(valid.Year())}
	}
	customers["acct_late"] = "cus_late"
	cards["cus_late"] = &payment.Card{ExpMonth: 1, ExpYear: 2020}

	expectListCarded(mock, zeroUUID, full...)
	expectListCarded(mock, full[len(full)-1], "acct_late")
	mock.ExpectExec("has_payment_method").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_late").
		WillReturnRows(cardedRow(billing.StatusActive, true))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &fakeDunningQueue{}
	w := &CardExpirySweepWorker{
		status:   billing.NewStatusStore(db, 7),
		cards:    fakeCardVault{cards: cards},
		accounts: fakeCardExpiryAccounts{customers: customers},
		queue:    q,
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[CardExpirySweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the sweep stopped at the first page: %v", err)
	}
	if len(q.suspended) != 1 || q.suspended[0] != "acct_late" {
		t.Errorf("suspended = %v, want [acct_late]: a full page of valid cards hid every account behind it", q.suspended)
	}
}

func TestCardExpirySweep_SuspendSurvivesAGatewayBudgetFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	expectListCarded(mock, zeroUUID, "acct_1")
	mock.ExpectExec("has_payment_method").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_1").
		WillReturnRows(cardedRow(billing.StatusActive, true))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	q := &fakeDunningQueue{budgeter: errors.New("river is down")}
	w := &CardExpirySweepWorker{
		status:   billing.NewStatusStore(db, 7),
		cards:    fakeCardVault{cards: map[string]*payment.Card{"cus_1": {ExpMonth: 1, ExpYear: 2020}}},
		accounts: fakeCardExpiryAccounts{customers: map[string]string{"acct_1": "cus_1"}},
		queue:    q,
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[CardExpirySweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	// The latch is already cleared, so the account has left every work set. A
	// suspend skipped here is never retried and its agents keep running.
	if len(q.suspended) != 1 || q.suspended[0] != "acct_1" {
		t.Errorf("suspended = %v, want [acct_1] despite the budget enqueue failing", q.suspended)
	}
	if len(q.notified) != 1 {
		t.Errorf("notified = %v, want one send", q.notified)
	}
}
