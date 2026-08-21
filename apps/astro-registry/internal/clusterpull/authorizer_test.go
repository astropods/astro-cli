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

// isPrimary must treat the literal "primary" sentinel (no cluster-config: the
// primary cluster has no clusters row) and the configured default cluster's
// real id (cluster-config boot sync: the primary is just another row, and its
// CPCs carry that real id, never the sentinel) as the same thing — this is
// exactly the case that broke ResolveHomedAccount for every unbound account
// once boot sync started routing the default cluster under its real id.
func TestIsPrimary(t *testing.T) {
	az := NewAuthorizer(nil, "", "preview-managed-eks")

	if !az.isPrimary(PrimaryClusterID) {
		t.Error("literal primary sentinel: want isPrimary=true")
	}
	if !az.isPrimary("preview-managed-eks") {
		t.Error("configured default cluster id: want isPrimary=true")
	}
	if az.isPrimary("some-other-cluster") {
		t.Error("unrelated additional cluster: want isPrimary=false")
	}

	azNoDefault := NewAuthorizer(nil, "", "")
	if azNoDefault.isPrimary("preview-managed-eks") {
		t.Error("no DEFAULT_CLUSTER_ID configured: want isPrimary=false for any non-sentinel id")
	}
}

func TestResolveHomedAccount(t *testing.T) {
	const defaultCluster = "preview-managed-eks"

	tests := []struct {
		name       string
		namespace  string
		clusterID  string
		wantColumn string
		bound      bool
		found      bool
		wantID     string
		wantHomed  bool
	}{
		{
			name:       "the default cluster needs no binding",
			namespace:  "acme",
			clusterID:  defaultCluster,
			wantColumn: "name",
			found:      true,
			wantID:     "acct-1",
			wantHomed:  true,
		},
		{
			name:       "the primary sentinel needs no binding",
			namespace:  "acme",
			clusterID:  PrimaryClusterID,
			wantColumn: "name",
			found:      true,
			wantID:     "acct-1",
			wantHomed:  true,
		},
		{
			name:       "an additional cluster the account is bound to",
			namespace:  "acme",
			clusterID:  "cluster-a",
			wantColumn: "name",
			found:      true,
			bound:      true,
			wantID:     "acct-1",
			wantHomed:  true,
		},
		{
			name:       "an additional cluster the account is not bound to",
			namespace:  "acme",
			clusterID:  "cluster-b",
			wantColumn: "name",
			found:      true,
			wantID:     "acct-1",
			wantHomed:  false,
		},
		{
			name:       "a uuid namespace looks up by id",
			namespace:  "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			clusterID:  "cluster-a",
			wantColumn: "id",
			found:      true,
			bound:      true,
			wantID:     "acct-1",
			wantHomed:  true,
		},
		{
			name:       "unknown account",
			namespace:  "nope",
			clusterID:  "cluster-a",
			wantColumn: "name",
			found:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close() //nolint:errcheck

			q := mock.ExpectQuery(`WHERE a\.`+tc.wantColumn+` = \$1`).WithArgs(tc.namespace, tc.clusterID)
			if tc.found {
				q.WillReturnRows(sqlmock.NewRows([]string{"id", "exists"}).AddRow("acct-1", tc.bound))
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
