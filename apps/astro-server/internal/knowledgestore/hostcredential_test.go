package knowledgestore

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

// fakeHostKMS is an identity KMS: the encrypted data key is the plaintext key,
// so Decrypt returns it unchanged. Enough to exercise the encrypt/decrypt
// round-trip without real KMS.
type fakeHostKMS struct{ key []byte }

func (f *fakeHostKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	cp := func() []byte { b := make([]byte, len(f.key)); copy(b, f.key); return b }
	return &kms.GenerateDataKeyOutput{Plaintext: cp(), CiphertextBlob: cp()}, nil
}

func (f *fakeHostKMS) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	plain := make([]byte, len(params.CiphertextBlob))
	copy(plain, params.CiphertextBlob)
	return &kms.DecryptOutput{Plaintext: plain}, nil
}

// errHostKMS fails every Decrypt — models KMS being unreachable / denied.
type errHostKMS struct{}

func (errHostKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return nil, errors.New("kms unavailable")
}

func (errHostKMS) Decrypt(_ context.Context, _ *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return nil, errors.New("kms decrypt denied")
}

func randHostKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

// TestEncryptHostCredential_RoundTrip: the HOST credential is keyed "HOST",
// encrypted (carries a nonce), and decrypts back to the host under the store's
// existing data key.
func TestEncryptHostCredential_RoundTrip(t *testing.T) {
	key := randHostKey(t)
	fk := &fakeHostKMS{key: key}
	store := &KnowledgeStore{ID: "s1", EncryptedDataKey: key}
	const dns = "vpce-0abc.vpce-svc-0def.us-east-1.vpce.amazonaws.com"

	cred, err := EncryptHostCredential(context.Background(), envelope.NewVault(fk, ""), store, dns)
	if err != nil {
		t.Fatalf("EncryptHostCredential: %v", err)
	}
	if cred.Key != HostCredentialKey {
		t.Errorf("cred.Key: got %q, want %q", cred.Key, HostCredentialKey)
	}
	if len(cred.Nonce) == 0 {
		t.Error("expected an encrypted value (non-empty nonce), got plaintext")
	}
	if string(cred.ValueEncrypted) == dns {
		t.Error("value stored in plaintext; expected ciphertext")
	}

	dec, err := envelope.NewDecryptor(context.Background(), fk, store.EncryptedDataKey)
	if err != nil {
		t.Fatalf("NewDecryptor: %v", err)
	}
	got, err := dec.Decrypt(cred.ValueEncrypted, cred.Nonce)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != dns {
		t.Errorf("decrypted host: got %q, want %q", got, dns)
	}
}

// TestEncryptHostCredential_KMSError: a KMS failure surfaces as an error rather
// than a silently-bad credential.
func TestEncryptHostCredential_KMSError(t *testing.T) {
	store := &KnowledgeStore{ID: "s1", EncryptedDataKey: randHostKey(t)}
	if _, err := EncryptHostCredential(context.Background(), envelope.NewVault(errHostKMS{}, ""), store, "host"); err == nil {
		t.Error("expected error when KMS Decrypt fails")
	}
}

// TestEncryptHostCredential_UniqueNonce: re-encrypting the same host yields a
// distinct nonce, so a rewrite never reuses one.
func TestEncryptHostCredential_UniqueNonce(t *testing.T) {
	fk := &fakeHostKMS{key: randHostKey(t)}
	store := &KnowledgeStore{ID: "s1", EncryptedDataKey: fk.key}

	c1, err := EncryptHostCredential(context.Background(), envelope.NewVault(fk, ""), store, "host")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := EncryptHostCredential(context.Background(), envelope.NewVault(fk, ""), store, "host")
	if err != nil {
		t.Fatal(err)
	}
	if string(c1.Nonce) == string(c2.Nonce) {
		t.Error("nonce reused across rewrites")
	}
}
