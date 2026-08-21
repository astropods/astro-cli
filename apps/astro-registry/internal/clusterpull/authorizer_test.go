package clusterpull

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func hashHex(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// Primary-cluster authentication uses the configured hash and touches no DB,
// so a nil *sql.DB is safe here.
func TestAuthenticate_Primary(t *testing.T) {
	az := NewAuthorizer(nil, hashHex("correct-secret"), "")
	ctx := context.Background()

	if ok, err := az.Authenticate(ctx, PrimaryClusterID, "correct-secret"); err != nil || !ok {
		t.Fatalf("correct secret: want ok=true err=nil, got ok=%v err=%v", ok, err)
	}
	if ok, err := az.Authenticate(ctx, PrimaryClusterID, "wrong-secret"); err != nil || ok {
		t.Fatalf("wrong secret: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

// An unset PRIMARY_PULL_KEY_HASH must fail closed, not authenticate everything.
func TestAuthenticate_Primary_Unconfigured(t *testing.T) {
	az := NewAuthorizer(nil, "", "")
	if ok, err := az.Authenticate(context.Background(), PrimaryClusterID, "anything"); err != nil || ok {
		t.Fatalf("unconfigured primary: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

// The literal "primary" sentinel (no cluster-config: the primary cluster has no
// clusters row) and the configured default cluster's real id (cluster-config
// boot sync: the primary is just another row, and its CPCs carry that real id)
// name the same cluster. account_clusters records the real id, so homing must
// resolve one to the other rather than treating them as different clusters.
func TestCanonicalClusterID(t *testing.T) {
	az := NewAuthorizer(nil, "", "preview-managed-eks")

	if got := az.canonicalClusterID(PrimaryClusterID); got != "preview-managed-eks" {
		t.Errorf("sentinel resolved to %q, want the configured default cluster", got)
	}
	if got := az.canonicalClusterID("preview-managed-eks"); got != "preview-managed-eks" {
		t.Errorf("configured default cluster resolved to %q, want it unchanged", got)
	}
	if got := az.canonicalClusterID("some-other-cluster"); got != "some-other-cluster" {
		t.Errorf("additional cluster resolved to %q, want it unchanged", got)
	}

	azNoDefault := NewAuthorizer(nil, "", "")
	if got := azNoDefault.canonicalClusterID(PrimaryClusterID); got != PrimaryClusterID {
		t.Errorf("unregistered primary resolved to %q, want the sentinel unchanged", got)
	}
}

func TestResolveHomedAccount(t *testing.T) {
	const defaultCluster = "preview-managed-eks"

	tests := []struct {
		name         string
		namespace    string
		clusterID    string
		wantColumn   string
		wantLookupID string
		bound        bool
		hasBindings  bool
		found        bool
		wantID       string
		wantHomed    bool
	}{
		{
			name:         "a cluster the account is bound to",
			namespace:    "acme",
			clusterID:    "cluster-a",
			wantColumn:   "name",
			wantLookupID: "cluster-a",
			found:        true,
			bound:        true,
			hasBindings:  true,
			wantID:       "acct-1",
			wantHomed:    true,
		},
		{
			name:         "a cluster the account is not bound to",
			namespace:    "acme",
			clusterID:    "cluster-b",
			wantColumn:   "name",
			wantLookupID: "cluster-b",
			found:        true,
			hasBindings:  true,
			wantID:       "acct-1",
			wantHomed:    false,
		},
		{
			name:         "an account confined elsewhere is refused on the primary",
			namespace:    "acme",
			clusterID:    defaultCluster,
			wantColumn:   "name",
			wantLookupID: defaultCluster,
			found:        true,
			hasBindings:  true,
			wantID:       "acct-1",
			wantHomed:    false,
		},
		{
			name:         "an unbound account routes to the primary",
			namespace:    "acme",
			clusterID:    defaultCluster,
			wantColumn:   "name",
			wantLookupID: defaultCluster,
			found:        true,
			wantID:       "acct-1",
			wantHomed:    true,
		},
		{
			name:         "an unbound account is not homed on an additional cluster",
			namespace:    "acme",
			clusterID:    "cluster-a",
			wantColumn:   "name",
			wantLookupID: "cluster-a",
			found:        true,
			wantID:       "acct-1",
			wantHomed:    false,
		},
		{
			name:         "the sentinel resolves to the configured default cluster",
			namespace:    "acme",
			clusterID:    PrimaryClusterID,
			wantColumn:   "name",
			wantLookupID: defaultCluster,
			found:        true,
			bound:        true,
			hasBindings:  true,
			wantID:       "acct-1",
			wantHomed:    true,
		},
		{
			name:         "a uuid namespace looks up by id",
			namespace:    "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			clusterID:    "cluster-a",
			wantColumn:   "id",
			wantLookupID: "cluster-a",
			found:        true,
			bound:        true,
			hasBindings:  true,
			wantID:       "acct-1",
			wantHomed:    true,
		},
		{
			name:         "unknown account",
			namespace:    "nope",
			clusterID:    "cluster-a",
			wantColumn:   "name",
			wantLookupID: "cluster-a",
			found:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close() //nolint:errcheck

			q := mock.ExpectQuery(`WHERE a\.`+tc.wantColumn+` = \$1`).WithArgs(tc.namespace, tc.wantLookupID)
			if tc.found {
				q.WillReturnRows(sqlmock.NewRows([]string{"id", "bound", "has_bindings"}).
					AddRow("acct-1", tc.bound, tc.hasBindings))
			} else {
				q.WillReturnError(sql.ErrNoRows)
			}

			az := NewAuthorizer(db, "", defaultCluster)
			id, homed, err := az.ResolveHomedAccount(context.Background(), tc.namespace, tc.clusterID)
			if err != nil {
				t.Fatalf("ResolveHomedAccount: %v", err)
			}
			if id != tc.wantID || homed != tc.wantHomed {
				t.Fatalf("got (%q, %v), want (%q, %v)", id, homed, tc.wantID, tc.wantHomed)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("expected a lookup by %s: %v", tc.wantColumn, err)
			}
		})
	}
}

// An unregistered primary has no clusters row, so it can never appear in
// account_clusters and an exhaustive check would refuse every bound account.
func TestResolveHomedAccount_UnregisteredPrimaryIgnoresBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`WHERE a\.name = \$1`).
		WithArgs("acme", PrimaryClusterID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bound", "has_bindings"}).
			AddRow("acct-1", false, true))

	az := NewAuthorizer(db, "", "")
	id, homed, err := az.ResolveHomedAccount(context.Background(), "acme", PrimaryClusterID)
	if err != nil {
		t.Fatalf("ResolveHomedAccount: %v", err)
	}
	if id != "acct-1" || !homed {
		t.Fatalf("got (%q, %v), want (acct-1, true)", id, homed)
	}
}

// Whether an account has any bindings decides the unbound case, and it is read
// in SQL, so a stubbed boolean cannot show the query asks for it.
func TestResolveHomedAccountQueryReadsBothBindingFacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery(`EXISTS \(SELECT 1 FROM account_clusters ac WHERE ac\.account_id = a\.id AND ac\.cluster_id = \$2\),\s+EXISTS \(SELECT 1 FROM account_clusters ac WHERE ac\.account_id = a\.id\)`).
		WithArgs("acme", "cluster-a").
		WillReturnRows(sqlmock.NewRows([]string{"id", "bound", "has_bindings"}).AddRow("acct-1", true, true))

	az := NewAuthorizer(db, "", "preview-managed-eks")
	if _, _, err := az.ResolveHomedAccount(context.Background(), "acme", "cluster-a"); err != nil {
		t.Fatalf("ResolveHomedAccount: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("query did not read both binding facts: %v", err)
	}
}
