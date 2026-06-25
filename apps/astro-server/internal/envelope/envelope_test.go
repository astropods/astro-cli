package envelope

import (
	"context"
	"crypto/aes"
	"crypto/rand"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// mockKMS implements KMSClient for testing.
type mockKMS struct {
	dataKey []byte // 32-byte AES key
}

func (m *mockKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	// Return the same key as both plaintext and "encrypted" (in prod, encrypted is opaque)
	plain := make([]byte, len(m.dataKey))
	copy(plain, m.dataKey)
	cipher := make([]byte, len(m.dataKey))
	copy(cipher, m.dataKey)
	return &kms.GenerateDataKeyOutput{
		KeyId:          strPtr("arn:aws:kms:us-east-1:123:key/test"),
		Plaintext:      plain,
		CiphertextBlob: cipher,
	}, nil
}

func (m *mockKMS) Decrypt(_ context.Context, params *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	// "Decrypt" the data key (in the mock, it's already plaintext)
	plain := make([]byte, len(params.CiphertextBlob))
	copy(plain, params.CiphertextBlob)
	return &kms.DecryptOutput{
		KeyId:     strPtr("arn:aws:kms:us-east-1:123:key/test"),
		Plaintext: plain,
	}, nil
}

func strPtr(s string) *string { return &s }

func TestEncryptDecryptRoundTrip(t *testing.T) {
	// Generate a random 32-byte key for the mock
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	mock := &mockKMS{dataKey: key}
	ctx := context.Background()

	// Create encryptor
	enc, err := NewEncryptor(ctx, mock, "arn:aws:kms:us-east-1:123:key/test")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	// Encrypt some values
	values := []string{
		"secret-api-key-123",
		"another-secret",
		"",
		"a longer secret value with special chars: !@#$%^&*()",
	}

	type encrypted struct {
		ciphertext []byte
		nonce      []byte
	}
	var results []encrypted

	for _, v := range values {
		ct, nonce, err := enc.Encrypt([]byte(v))
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", v, err)
		}
		if len(nonce) != aes.BlockSize-4 { // GCM nonce is 12 bytes
			t.Fatalf("unexpected nonce size: %d", len(nonce))
		}
		results = append(results, encrypted{ct, nonce})
	}

	// Create decryptor from the encrypted data key
	dec, err := NewDecryptor(ctx, mock, enc.EncryptedDataKey)
	if err != nil {
		t.Fatalf("NewDecryptor: %v", err)
	}

	// Decrypt and verify round-trip
	for i, v := range values {
		plaintext, err := dec.Decrypt(results[i].ciphertext, results[i].nonce)
		if err != nil {
			t.Fatalf("Decrypt(%d): %v", i, err)
		}
		if string(plaintext) != v {
			t.Errorf("Decrypt(%d): got %q, want %q", i, plaintext, v)
		}
	}
}

func TestUniqueNonces(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	mock := &mockKMS{dataKey: key}

	enc, err := NewEncryptor(context.Background(), mock, "arn:aws:kms:us-east-1:123:key/test")
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt the same value twice — nonces must differ
	_, nonce1, _ := enc.Encrypt([]byte("same"))
	_, nonce2, _ := enc.Encrypt([]byte("same"))

	if string(nonce1) == string(nonce2) {
		t.Error("nonces should be unique per encryption")
	}
}

// TestDecryptDataKey_ReEncryptUnderExistingKey verifies the rewrite-in-place
// pattern: a value encrypted via an encryptor rebuilt from DecryptDataKey
// decrypts under the same encrypted data key, alongside the original values.
func TestDecryptDataKey_ReEncryptUnderExistingKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	mock := &mockKMS{dataKey: key}
	ctx := context.Background()

	// Original encryptor produces the store's encrypted data key + a value.
	enc, err := NewEncryptor(ctx, mock, "arn:aws:kms:us-east-1:123:key/test")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	origCT, origNonce, _ := enc.Encrypt([]byte("original-secret"))

	// Rebuild an encryptor from the decrypted data key and encrypt a new value.
	plaintextKey, err := DecryptDataKey(ctx, mock, enc.EncryptedDataKey)
	if err != nil {
		t.Fatalf("DecryptDataKey: %v", err)
	}
	enc2, err := NewEncryptorFromPlaintext(plaintextKey, enc.EncryptedDataKey, "")
	if err != nil {
		t.Fatalf("NewEncryptorFromPlaintext: %v", err)
	}
	newCT, newNonce, _ := enc2.Encrypt([]byte("rewritten-host"))

	// A single decryptor (the store's data key) decrypts both old and new.
	dec, err := NewDecryptor(ctx, mock, enc.EncryptedDataKey)
	if err != nil {
		t.Fatalf("NewDecryptor: %v", err)
	}
	if got, _ := dec.Decrypt(origCT, origNonce); string(got) != "original-secret" {
		t.Errorf("original value: got %q", got)
	}
	if got, _ := dec.Decrypt(newCT, newNonce); string(got) != "rewritten-host" {
		t.Errorf("rewritten value: got %q", got)
	}
}
