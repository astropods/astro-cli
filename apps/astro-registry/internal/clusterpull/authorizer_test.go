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
	az := NewAuthorizer(nil, hashHex("correct-secret"))
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
	az := NewAuthorizer(nil, "")
	if ok, err := az.Authenticate(context.Background(), PrimaryClusterID, "anything"); err != nil || ok {
		t.Fatalf("unconfigured primary: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

// A malformed namespace must short-circuit before the DB query — with a nil
// *sql.DB this test would panic on QueryRowContext if the UUID guard regressed.
func TestHomedHere_MalformedUUID_ShortCircuits(t *testing.T) {
	az := NewAuthorizer(nil, "")
	if ok, err := az.HomedHere(context.Background(), "not-a-uuid", PrimaryClusterID); err != nil || ok {
		t.Fatalf("malformed uuid: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}
