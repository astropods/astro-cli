//go:build integration

package e2e

import (
	"database/sql"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	_ "github.com/lib/pq"
)

// The signup credit is one per person. Deleting the account that holds the
// claim must not return it, or the grant is farmed by deleting and signing up
// again as the same person.
func TestSignupCredit_NotRestoredByDeletingTheAccount(t *testing.T) {
	db := testDB(t)
	store := account.NewAccountStore(db)

	userID := "user-credit-e2e-" + deployid.New()
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM billing_credit_grants WHERE user_id = $1`, userID)
	})

	first := createCreditTestAccount(t, db, store, userID)
	claimed, err := store.ClaimSignupCredit(userID, first)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("first account did not take the signup credit")
	}

	// A provisioning retry on the holding account keeps its credit.
	claimed, err = store.ClaimSignupCredit(userID, first)
	if err != nil {
		t.Fatalf("repeat claim: %v", err)
	}
	if !claimed {
		t.Error("repeat claim by the holder reported no credit")
	}

	second := createCreditTestAccount(t, db, store, userID)
	claimed, err = store.ClaimSignupCredit(userID, second)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Error("a second account by the same person took the credit again")
	}

	// Hard-delete both accounts, as the purge worker does after retention.
	for _, id := range []string{first, second} {
		if _, err := db.Exec(`DELETE FROM accounts WHERE id = $1`, id); err != nil {
			t.Fatalf("purge account %s: %v", id, err)
		}
	}

	var holder string
	err = db.QueryRow(`SELECT account_id FROM billing_credit_grants WHERE user_id = $1`, userID).Scan(&holder)
	if err != nil {
		t.Fatalf("the claim did not survive the purge: %v", err)
	}
	if holder != first {
		t.Errorf("claim holder = %s, want %s", holder, first)
	}

	third := createCreditTestAccount(t, db, store, userID)
	claimed, err = store.ClaimSignupCredit(userID, third)
	if err != nil {
		t.Fatalf("post-delete claim: %v", err)
	}
	if claimed {
		t.Error("signing up again after deleting the account took a second credit")
	}
}

func createCreditTestAccount(t *testing.T, db *sql.DB, store *account.AccountStore, userID string) string {
	t.Helper()
	acct, err := store.Create("credit-e2e-"+deployid.New(), "personal", userID, "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM accounts WHERE id = $1`, acct.ID) })
	return acct.ID
}
