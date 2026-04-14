package knowledgestore

import (
	"strings"
	"testing"
)

func TestValidateStorageSize(t *testing.T) {
	valid := []string{"10Gi", "20Gi", "500Mi", "1Ti", "5G", "1024"}
	for _, s := range valid {
		if err := ValidateStorageSize(s); err != nil {
			t.Errorf("ValidateStorageSize(%q) unexpected error: %v", s, err)
		}
	}

	invalid := []struct {
		size string
		msg  string
	}{
		{"", "empty"},
		{"0Gi", "zero"},
		{"-10Gi", "negative"},
		{"10gigabytes", "invalid unit"},
		{"abc", "not a quantity"},
	}
	for _, tt := range invalid {
		if err := ValidateStorageSize(tt.size); err == nil {
			t.Errorf("ValidateStorageSize(%q) expected error for %s, got nil", tt.size, tt.msg)
		}
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

func TestGenerateCredentials_Postgres(t *testing.T) {
	creds, err := GenerateCredentials("postgres")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if creds[key] == "" {
			t.Errorf("expected non-empty %s", key)
		}
	}
	if creds["POSTGRES_USER"] != "astro" {
		t.Errorf("expected POSTGRES_USER=astro, got %q", creds["POSTGRES_USER"])
	}
	if creds["POSTGRES_DB"] != "astro" {
		t.Errorf("expected POSTGRES_DB=astro, got %q", creds["POSTGRES_DB"])
	}
	// Password should be random hex — 32 chars (16 bytes)
	if len(creds["POSTGRES_PASSWORD"]) != 32 {
		t.Errorf("expected 32-char password, got %d", len(creds["POSTGRES_PASSWORD"]))
	}
}

func TestGenerateCredentials_Qdrant(t *testing.T) {
	creds, err := GenerateCredentials("qdrant")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds["QDRANT__SERVICE__API_KEY"] == "" {
		t.Error("expected non-empty QDRANT__SERVICE__API_KEY")
	}
	// 24 bytes → 48 hex chars
	if len(creds["QDRANT__SERVICE__API_KEY"]) != 48 {
		t.Errorf("expected 48-char key, got %d", len(creds["QDRANT__SERVICE__API_KEY"]))
	}
}

func TestGenerateCredentials_Redis(t *testing.T) {
	creds, err := GenerateCredentials("redis")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds["REDIS_PASSWORD"] == "" {
		t.Error("expected non-empty REDIS_PASSWORD")
	}
}

func TestGenerateCredentials_Neo4j(t *testing.T) {
	creds, err := GenerateCredentials("neo4j")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := creds["NEO4J_AUTH"]
	if !strings.HasPrefix(auth, "neo4j/") {
		t.Errorf("expected NEO4J_AUTH to start with 'neo4j/', got %q", auth)
	}
	if len(auth) <= len("neo4j/") {
		t.Error("expected non-empty password in NEO4J_AUTH")
	}
}

func TestGenerateCredentials_Unknown(t *testing.T) {
	creds, err := GenerateCredentials("pinecone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("expected empty creds for unknown provider, got %v", creds)
	}
}

func TestGenerateCredentials_RandomEachCall(t *testing.T) {
	a, _ := GenerateCredentials("postgres")
	b, _ := GenerateCredentials("postgres")
	if a["POSTGRES_PASSWORD"] == b["POSTGRES_PASSWORD"] {
		t.Error("expected different passwords on each call")
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
