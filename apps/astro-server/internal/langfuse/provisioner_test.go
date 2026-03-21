package langfuse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestComputeFastHash(t *testing.T) {
	// Reference: sha256(secretKey + hex(sha256(salt)))
	ref := func(key, salt string) string {
		saltDigest := sha256.Sum256([]byte(salt))
		saltHex := hex.EncodeToString(saltDigest[:])
		final := sha256.Sum256([]byte(key + saltHex))
		return hex.EncodeToString(final[:])
	}

	tests := []struct {
		name string
		key  string
		salt string
	}{
		{"known inputs", "mykey", "mysalt"},
		{"empty salt", "mykey", ""},
		{"empty key", "", "mysalt"},
		{"both empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeFastHash(tt.key, tt.salt)
			want := ref(tt.key, tt.salt)
			if got != want {
				t.Errorf("computeFastHash(%q, %q) = %s, want %s", tt.key, tt.salt, got, want)
			}
			if len(got) != 64 {
				t.Errorf("expected 64-char hex, got length %d", len(got))
			}
		})
	}
}

func TestGenerateCUID_Format(t *testing.T) {
	t.Run("length is 24", func(t *testing.T) {
		id := generateCUID()
		if len(id) != 24 {
			t.Fatalf("expected length 24, got %d (%q)", len(id), id)
		}
	})

	t.Run("starts with lowercase letter", func(t *testing.T) {
		for i := 0; i < 200; i++ {
			id := generateCUID()
			if id[0] < 'a' || id[0] > 'z' {
				t.Fatalf("expected first char a-z, got %q in %q", id[0], id)
			}
		}
	})

	t.Run("all lowercase alphanumeric", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			id := generateCUID()
			for j, c := range id {
				if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
					t.Fatalf("char %d (%q) in %q is not lowercase alphanumeric", j, string(c), id)
				}
			}
		}
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			id := generateCUID()
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate CUID after %d calls: %q", i, id)
			}
			seen[id] = struct{}{}
		}
	})
}

func TestDecryptSecretKey(t *testing.T) {
	ctx := context.Background()

	t.Run("plaintext fallback when no encryption", func(t *testing.T) {
		creds := &AccountLangfuse{
			SecretKey:        "sk-lf-plaintext-key",
			EncryptedDataKey: nil,
			Nonce:            nil,
		}
		got, err := decryptSecretKey(ctx, nil, creds)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "sk-lf-plaintext-key" {
			t.Fatalf("got %q, want plaintext key", got)
		}
	})

	t.Run("plaintext fallback with empty slices", func(t *testing.T) {
		creds := &AccountLangfuse{
			SecretKey:        "sk-lf-another-key",
			EncryptedDataKey: []byte{},
			Nonce:            []byte{},
		}
		got, err := decryptSecretKey(ctx, nil, creds)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "sk-lf-another-key" {
			t.Fatalf("got %q, want plaintext key", got)
		}
	})

	t.Run("error when encrypted but no kms client", func(t *testing.T) {
		creds := &AccountLangfuse{
			SecretKey:        "encrypted-blob",
			EncryptedDataKey: []byte("some-encrypted-data-key"),
			Nonce:            []byte("some-nonce"),
		}
		_, err := decryptSecretKey(ctx, nil, creds)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		want := "KMS client required to decrypt Langfuse secret key"
		if err.Error() != want {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	})
}
