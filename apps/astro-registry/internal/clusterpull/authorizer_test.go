package clusterpull

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
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

// ResolveHomedAccount is DB-backed (name/id lookup + homing) and is exercised
// via the /token isolation tests (handlers) and preview integration; there is
// no sqlmock in this module to unit-test the query here.
