//go:build integration

package riverqueue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// fakeKMS is an in-memory KMSClient: the encrypted data key is the plaintext
// key itself, so Decrypt is the identity. Enough to exercise credential
// encryption without real KMS.
type fakeKMS struct{ key []byte }

func (f *fakeKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	cp := func() []byte { b := make([]byte, len(f.key)); copy(b, f.key); return b }
	return &kms.GenerateDataKeyOutput{Plaintext: cp(), CiphertextBlob: cp()}, nil
}

func (f *fakeKMS) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	plain := make([]byte, len(params.CiphertextBlob))
	copy(plain, params.CiphertextBlob)
	return &kms.DecryptOutput{Plaintext: plain}, nil
}

func randKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

// seedExternalStore creates an account + external store and saves the given
// plaintext credentials encrypted under a fake-KMS data key. Returns the store
// ID and the data key (which doubles as the encrypted data key under the fake).
func seedExternalStore(t *testing.T, db *sql.DB, s *knowledgestore.Store, creds map[string]string) (string, []byte) {
	t.Helper()
	key := randKey(t)

	accountID := ensureKRAccount(t, db)

	storeID := deployid.New()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM knowledge_stores WHERE id = $1`, storeID) })

	if _, err := s.Create(knowledgestore.CreateParams{
		ID:               storeID,
		AccountID:        accountID,
		Name:             "kr-" + storeID,
		ARN:              "arn:knowledge:test-kr-account:kr-" + storeID,
		Provider:         "postgres",
		Status:           knowledgestore.StatusReady,
		EncryptedDataKey: key,
	}); err != nil {
		t.Fatalf("create store: %v", err)
	}

	enc, err := envelope.NewEncryptorFromPlaintext(key, key, "")
	if err != nil {
		t.Fatalf("encryptor: %v", err)
	}
	encrypted, err := knowledgestore.EncryptCredentials(enc, creds)
	if err != nil {
		t.Fatalf("encrypt creds: %v", err)
	}
	if err := s.SaveCredentials(storeID, encrypted); err != nil {
		t.Fatalf("save creds: %v", err)
	}
	return storeID, key
}

// ensureKRAccount returns a stable test account ID.
func ensureKRAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`
		WITH acct AS (
			INSERT INTO accounts (name, type, owner_user_id) VALUES ('test-kr-account', 'personal', 'test-owner')
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		), member AS (
			INSERT INTO account_members (account_id, user_id) SELECT id, 'test-owner' FROM acct
			ON CONFLICT DO NOTHING
		)
		SELECT id FROM acct`).Scan(&id); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return id
}

// decryptCreds returns the store's credentials as a plaintext map.
func decryptCreds(t *testing.T, s *knowledgestore.Store, storeID string, kms envelope.KMSClient, edk []byte) map[string]string {
	t.Helper()
	raw, err := s.GetCredentials(storeID)
	if err != nil {
		t.Fatalf("get creds: %v", err)
	}
	dec, err := envelope.NewDecryptor(context.Background(), kms, edk)
	if err != nil {
		t.Fatalf("decryptor: %v", err)
	}
	out := map[string]string{}
	for _, c := range raw {
		plain, err := dec.Decrypt(c.ValueEncrypted, c.Nonce)
		if err != nil {
			t.Fatalf("decrypt %s: %v", c.Key, err)
		}
		out[c.Key] = string(plain)
	}
	return out
}

// TestPersistResolvedHost_Integration: rewriting HOST to the resolved DNS
// updates only the HOST row — the other credentials decrypt unchanged under the
// same data key (proving no re-key and no collateral overwrite).
func TestPersistResolvedHost_Integration(t *testing.T) {
	db := integrationDB(t)
	store := knowledgestore.NewStore(db)

	const vpceService = "com.amazonaws.vpce.us-east-1.vpce-svc-0def456"
	const resolvedDNS = "vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com"
	storeID, key := seedExternalStore(t, db, store, map[string]string{
		"HOST":     vpceService,
		"PORT":     "5432",
		"USERNAME": "astro",
		"PASSWORD": "secret123",
	})

	w := &KnowledgeReconcileWorker{ksStore: store, log: logger.New("error", "json"), vault: envelope.NewVault(&fakeKMS{key: key}, "")}
	if err := w.persistResolvedHost(context.Background(), storeID, resolvedDNS); err != nil {
		t.Fatalf("persistResolvedHost: %v", err)
	}

	got := decryptCreds(t, store, storeID, &fakeKMS{key: key}, key)
	if got["HOST"] != resolvedDNS {
		t.Errorf("HOST: got %q, want resolved DNS %q", got["HOST"], resolvedDNS)
	}
	for k, want := range map[string]string{"PORT": "5432", "USERNAME": "astro", "PASSWORD": "secret123"} {
		if got[k] != want {
			t.Errorf("%s: got %q, want unchanged %q", k, got[k], want)
		}
	}
}

// TestPersistResolvedHost_Integration_NoDataKey: a store without an encrypted
// data key (KMS off — no persisted external credentials) is a no-op, not an error.
func TestPersistResolvedHost_Integration_NoDataKey(t *testing.T) {
	db := integrationDB(t)
	store := knowledgestore.NewStore(db)

	accountID := ensureKRAccount(t, db)
	storeID := deployid.New()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM knowledge_stores WHERE id = $1`, storeID) })
	if _, err := store.Create(knowledgestore.CreateParams{
		ID: storeID, AccountID: accountID, Name: "kr-nodk-" + storeID,
		ARN:      "arn:knowledge:test-kr-account:kr-nodk-" + storeID,
		Provider: "postgres", Status: knowledgestore.StatusReady,
	}); err != nil {
		t.Fatalf("create store: %v", err)
	}

	w := &KnowledgeReconcileWorker{ksStore: store, log: logger.New("error", "json"), vault: envelope.NewVault(&fakeKMS{key: randKey(t)}, "")}
	if err := w.persistResolvedHost(context.Background(), storeID, "anything"); err != nil {
		t.Errorf("expected no-op for store without data key, got %v", err)
	}
	if creds, _ := store.GetCredentials(storeID); len(creds) != 0 {
		t.Errorf("expected no credentials persisted, got %d", len(creds))
	}
}
