package billing

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSetPayLink_UpsertsTheGivenURL(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec("pay_link = EXCLUDED.pay_link").
		WithArgs("acct_1", "https://pay.stripe.com/invoice/in_1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := NewStatusStore(db, 7).SetPayLink(context.Background(), "acct_1", "https://pay.stripe.com/invoice/in_1"); err != nil {
		t.Fatalf("SetPayLink: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClearStalePayLink_ClearsOnlyWhenTheStoredLinkDiffers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec(`pay_link IS NOT NULL AND pay_link <> \$2`).
		WithArgs("acct_1", "https://pay.stripe.com/invoice/in_new").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := NewStatusStore(db, 7).ClearStalePayLink(context.Background(), "acct_1", "https://pay.stripe.com/invoice/in_new"); err != nil {
		t.Fatalf("ClearStalePayLink: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestClearStalePayLink_TreatsAnUnnamedInvoiceAsClearing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectExec(`pay_link IS NOT NULL AND pay_link <> \$2`).
		WithArgs("acct_1", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := NewStatusStore(db, 7).ClearStalePayLink(context.Background(), "acct_1", ""); err != nil {
		t.Fatalf("ClearStalePayLink: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
