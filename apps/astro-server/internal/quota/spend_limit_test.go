package quota

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func ceilingDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func TestIsRequestable_CoversTheSpendLimitButNotAsAResource(t *testing.T) {
	if IsResource(KeySpendLimit) {
		t.Error("the spend limit is not a count-enforced resource")
	}
	if !IsRequestable(KeySpendLimit) {
		t.Error("the spend limit has to be requestable")
	}
	if !IsRequestable(ResourceBlueprints) {
		t.Error("every resource stays requestable")
	}
	if IsRequestable("compute") {
		t.Error("a metered feature is gated by billing, not by a request")
	}
}

func TestSpendCeilingUSD_NoGrantIsTheSelfServeCeiling(t *testing.T) {
	db, mock := ceilingDB(t)
	mock.ExpectQuery("account_limits").WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))

	got, err := SpendCeilingUSD(context.Background(), db, "acct-1")
	if err != nil {
		t.Fatalf("SpendCeilingUSD: %v", err)
	}
	if got != billing.MaxSelfServeSpendUSD {
		t.Errorf("ceiling = %v, want %v", got, billing.MaxSelfServeSpendUSD)
	}
}

func TestSpendCeilingUSD_AnApprovedGrantRaisesIt(t *testing.T) {
	db, mock := ceilingDB(t)
	mock.ExpectQuery("account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(5000))

	got, err := SpendCeilingUSD(context.Background(), db, "acct-1")
	if err != nil {
		t.Fatalf("SpendCeilingUSD: %v", err)
	}
	if got != 5000 {
		t.Errorf("ceiling = %v, want the granted 5000", got)
	}
}

func TestSpendCeilingUSD_AGrantNeverLowersIt(t *testing.T) {
	for _, granted := range []int64{-1, 0, 20} {
		db, mock := ceilingDB(t)
		mock.ExpectQuery("account_limits").
			WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(granted))

		got, err := SpendCeilingUSD(context.Background(), db, "acct-1")
		if err != nil {
			t.Fatalf("SpendCeilingUSD(%d): %v", granted, err)
		}
		if got != billing.MaxSelfServeSpendUSD {
			t.Errorf("ceiling for granted %d = %v, want %v", granted, got, billing.MaxSelfServeSpendUSD)
		}
	}
}

func TestSpendCeilingUSD_ReadFailureIsAnError(t *testing.T) {
	db, mock := ceilingDB(t)
	mock.ExpectQuery("account_limits").WillReturnError(sql.ErrConnDone)

	if _, err := SpendCeilingUSD(context.Background(), db, "acct-1"); err == nil {
		t.Fatal("want the read failure")
	}
}

func TestSpendCeilingUSD_NilDBIsTheSelfServeCeiling(t *testing.T) {
	got, err := SpendCeilingUSD(context.Background(), nil, "acct-1")
	if err != nil {
		t.Fatalf("SpendCeilingUSD: %v", err)
	}
	if got != billing.MaxSelfServeSpendUSD {
		t.Errorf("ceiling = %v, want %v", got, billing.MaxSelfServeSpendUSD)
	}
}
