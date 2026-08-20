package knowledgestore

import (
	"context"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

// TestEncryptCredentialsForStore_RoundTrip: values re-encrypted under a store's
// existing data key decrypt alongside its untouched credentials, and only the
// updated keys change.
func TestEncryptCredentialsForStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	kms, err := envelope.NewLocalKMSClient()
	if err != nil {
		t.Fatalf("NewLocalKMSClient: %v", err)
	}
	enc, err := envelope.NewEncryptor(ctx, kms, "")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	initial, err := EncryptCredentials(enc, map[string]string{
		"HOST": "old-host", "PORT": "5432", "DATABASE": "postgres",
		"USERNAME": "u", "PASSWORD": "oldpw",
	})
	if err != nil {
		t.Fatalf("EncryptCredentials: %v", err)
	}
	store := &KnowledgeStore{ID: "s1", Mode: ModeExternal, EncryptedDataKey: enc.EncryptedDataKey}

	// Re-encrypt an update (password + host) under the same data key.
	updated, err := EncryptCredentialsForStore(ctx, envelope.NewVault(kms, ""), store, map[string]string{
		"PASSWORD": "newpw", "HOST": "new-host",
	})
	if err != nil {
		t.Fatalf("EncryptCredentialsForStore: %v", err)
	}

	// Simulate the SaveCredentials upsert: updated rows overwrite initial by key.
	byKey := map[string]Credential{}
	for _, c := range initial {
		byKey[c.Key] = c
	}
	for _, c := range updated {
		byKey[c.Key] = c
	}
	all := make([]Credential, 0, len(byKey))
	for _, c := range byKey {
		all = append(all, c)
	}

	got, err := DecryptCredentials(ctx, envelope.NewVault(kms, ""), store.EncryptedDataKey, all)
	if err != nil {
		t.Fatalf("DecryptCredentials: %v", err)
	}
	if got["PASSWORD"] != "newpw" {
		t.Errorf("PASSWORD = %q, want newpw", got["PASSWORD"])
	}
	if got["HOST"] != "new-host" {
		t.Errorf("HOST = %q, want new-host", got["HOST"])
	}
	if got["USERNAME"] != "u" || got["PORT"] != "5432" || got["DATABASE"] != "postgres" {
		t.Errorf("untouched creds changed: %+v", got)
	}
}

// TestRewriteCredentials_NoDataKeyNoop: a store with no data key (KMS off) has
// no persisted credentials to update, so RewriteCredentials is a no-op.
func TestRewriteCredentials_NoDataKeyNoop(t *testing.T) {
	s := &Store{}
	store := &KnowledgeStore{ID: "s1"} // no EncryptedDataKey
	if err := s.RewriteCredentials(context.Background(), nil, store, map[string]string{"PASSWORD": "x"}); err != nil {
		t.Fatalf("expected no-op nil, got %v", err)
	}
}

func TestValidateStoreName(t *testing.T) {
	valid := []string{
		"db", "pg-main", "my-store", "postgres-prod", "a", "db1",
		"store-123", strings.Repeat("a", 63),
	}
	for _, name := range valid {
		if err := ValidateStoreName(name); err != nil {
			t.Errorf("ValidateStoreName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []struct {
		name string
		msg  string
	}{
		{"", "empty"},
		{strings.Repeat("a", 64), "too long"},
		{"-db", "leading hyphen"},
		{"db-", "trailing hyphen"},
		{"my--store", "consecutive hyphens"},
		{"My-Store", "uppercase"},
		{"my store", "space"},
		{"my_store", "underscore"},
		{"my.store", "dot"},
		{"arn:knowledge", "colon"},
	}
	for _, tt := range invalid {
		if err := ValidateStoreName(tt.name); err == nil {
			t.Errorf("ValidateStoreName(%q) expected error for %s, got nil", tt.name, tt.msg)
		}
	}
}

// --- ExternalCredentialKeys ---

func TestExternalCredentialKeys(t *testing.T) {
	cases := []struct {
		provider string
		expected []string
	}{
		{"postgres", []string{"HOST", "PORT", "DATABASE", "USERNAME", "PASSWORD"}},
		{"qdrant", []string{"HOST", "PORT", "API_KEY"}},
		{"redis", []string{"HOST", "PORT", "PASSWORD"}},
		{"neo4j", []string{"HOST", "PORT", "USERNAME", "PASSWORD"}},
		{"pinecone", []string{"HOST", "API_KEY"}},
		{"mysql", []string{"HOST", "PORT", "DATABASE", "USERNAME", "PASSWORD"}},
		{"unknown-provider", []string{"HOST", "PORT"}},
	}

	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			keys := ExternalCredentialKeys(tc.provider)
			if len(keys) != len(tc.expected) {
				t.Fatalf("expected %d keys, got %d: %v", len(tc.expected), len(keys), keys)
			}
			for i, k := range keys {
				if k != tc.expected[i] {
					t.Errorf("key[%d]: expected %q, got %q", i, tc.expected[i], k)
				}
			}
		})
	}
}

// --- ValidateExternalCredentials ---

func TestValidateExternalCredentials_Valid(t *testing.T) {
	cases := []struct {
		provider string
		creds    map[string]string
	}{
		{"postgres", map[string]string{"HOST": "db.example.com", "PORT": "5432", "DATABASE": "mydb", "USERNAME": "app", "PASSWORD": "secret"}},
		{"qdrant", map[string]string{"HOST": "qdrant.example.com", "PORT": "6333", "API_KEY": "key123"}},
		{"redis", map[string]string{"HOST": "redis.example.com", "PORT": "6379", "PASSWORD": "pass"}},
		{"pinecone", map[string]string{"HOST": "index.pinecone.io", "API_KEY": "key123"}},
	}

	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			if err := ValidateExternalCredentials(tc.provider, tc.creds); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateExternalCredentials_MissingKey(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		creds    map[string]string
		missing  string
	}{
		{"postgres missing password", "postgres", map[string]string{"HOST": "h", "PORT": "5432", "DATABASE": "db", "USERNAME": "u"}, "PASSWORD"},
		{"postgres missing host", "postgres", map[string]string{"PORT": "5432", "DATABASE": "db", "USERNAME": "u", "PASSWORD": "p"}, "HOST"},
		{"qdrant missing api key", "qdrant", map[string]string{"HOST": "h", "PORT": "6333"}, "API_KEY"},
		{"redis missing password", "redis", map[string]string{"HOST": "h", "PORT": "6379"}, "PASSWORD"},
		{"pinecone missing host", "pinecone", map[string]string{"API_KEY": "k"}, "HOST"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateExternalCredentials(tc.provider, tc.creds)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.missing) {
				t.Errorf("expected error to mention %q, got: %v", tc.missing, err)
			}
		})
	}
}

func TestValidateExternalCredentials_EmptyValueTreatedAsMissing(t *testing.T) {
	creds := map[string]string{"HOST": "h", "PORT": "5432", "DATABASE": "db", "USERNAME": "u", "PASSWORD": ""}
	err := ValidateExternalCredentials("postgres", creds)
	if err == nil {
		t.Fatal("expected error for empty PASSWORD, got nil")
	}
}

// --- ResolveCredentials ---

func TestResolveCredentials_VaultRequired(t *testing.T) {
	store := &KnowledgeStore{Name: "my-db", EncryptedDataKey: []byte("key")}
	dbCreds := []Credential{{Key: "POSTGRES_USER", ValueEncrypted: []byte("enc"), Nonce: []byte("n")}}

	_, err := ResolveCredentials(context.Background(), store, dbCreds, nil)
	if err == nil {
		t.Fatal("expected error when no vault is configured")
	}
	if !strings.Contains(err.Error(), "no vault configured") {
		t.Errorf("error should name the missing vault, got: %v", err)
	}
}
