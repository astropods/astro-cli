package knowledgestore

import (
	"strings"
	"testing"
)

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
