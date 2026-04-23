package spec

import (
	"testing"
)

func TestParseReferences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantKind ReferenceKind
		wantName string
		wantEp   string
		wantAttr string
	}{
		{"model host", "${models.local_llm.host}", 1, RefModel, "local_llm", "", "host"},
		{"model port 4-part", "${models.local_llm.http.port}", 1, RefModel, "local_llm", "http", "port"},
		{"model url 4-part", "${models.local_llm.http.url}", 1, RefModel, "local_llm", "http", "url"},
		{"knowledge host", "${knowledge.docs.host}", 1, RefKnowledge, "docs", "", "host"},
		{"tool url", "${integrations.search.http.url}", 1, RefIntegration, "search", "http", "url"},
		{"variable", "${variables.API_KEY}", 1, RefVariable, "API_KEY", "", ""},
		{"source name", "${source.name}", 1, RefSource, "name", "", ""},
		{"source build", "${source.build}", 1, RefSource, "build", "", ""},
		{"no refs", "plain string", 0, "", "", "", ""},
		{"partial ref ignored", "${invalid}", 0, "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := ParseReferences(tt.input)
			if len(refs) != tt.wantLen {
				t.Fatalf("expected %d refs, got %d: %+v", tt.wantLen, len(refs), refs)
			}
			if tt.wantLen == 0 {
				return
			}
			ref := refs[0]
			if ref.Kind != tt.wantKind {
				t.Errorf("kind: expected %s, got %s", tt.wantKind, ref.Kind)
			}
			if ref.Name != tt.wantName {
				t.Errorf("name: expected %s, got %s", tt.wantName, ref.Name)
			}
			if ref.Endpoint != tt.wantEp {
				t.Errorf("endpoint: expected %q, got %q", tt.wantEp, ref.Endpoint)
			}
			if ref.Attribute != tt.wantAttr {
				t.Errorf("attribute: expected %s, got %s", tt.wantAttr, ref.Attribute)
			}
		})
	}
}

func TestParseReferences_MultipleInOneString(t *testing.T) {
	refs := ParseReferences("http://${models.llm.host}:${models.llm.http.port}")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].Attribute != "host" || refs[0].Endpoint != "" {
		t.Errorf("expected host (3-part) ref, got endpoint=%q attr=%q", refs[0].Endpoint, refs[0].Attribute)
	}
	if refs[1].Attribute != "port" || refs[1].Endpoint != "http" {
		t.Errorf("expected http.port (4-part) ref, got endpoint=%q attr=%q", refs[1].Endpoint, refs[1].Attribute)
	}
}

func TestExtractAllReferences(t *testing.T) {
	env := map[string]string{
		"LLM_URL":     "${models.llm.http.url}",
		"QDRANT_HOST": "${knowledge.docs.host}",
		"API_KEY":     "${variables.ANTHROPIC_API_KEY}",
		"STATIC":      "no_refs_here",
	}

	refs := ExtractAllReferences(env)
	if len(refs) != 3 {
		t.Errorf("expected 3 refs from env map, got %d", len(refs))
	}
}

func TestValidateReferences_Valid(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Models: map[string]DeploymentModel{
			"llm": {Image: "x", Endpoints: map[string]Endpoint{"http": {Port: 8080}}},
		},
		Knowledge: map[string]DeploymentKnowledge{
			"docs": {Image: "x", Endpoints: map[string]Endpoint{"http": {Port: 6333}}},
		},
		Integrations: map[string]DeploymentIntegration{
			"search": {Image: "x", Endpoints: map[string]Endpoint{"http": {Port: 3000}}},
		},
		Variables: map[string]Variable{
			"API_KEY": {Description: "key", Secret: true},
		},
	}

	refs := []Reference{
		{Raw: "${models.llm.host}", Kind: RefModel, Name: "llm", Attribute: "host"},
		{Raw: "${models.llm.http.port}", Kind: RefModel, Name: "llm", Endpoint: "http", Attribute: "port"},
		{Raw: "${models.llm.http.url}", Kind: RefModel, Name: "llm", Endpoint: "http", Attribute: "url"},
		{Raw: "${knowledge.docs.host}", Kind: RefKnowledge, Name: "docs", Attribute: "host"},
		{Raw: "${integrations.search.http.url}", Kind: RefIntegration, Name: "search", Endpoint: "http", Attribute: "url"},
		{Raw: "${variables.API_KEY}", Kind: RefVariable, Name: "API_KEY"},
		{Raw: "${source.name}", Kind: RefSource, Name: "name"},
		{Raw: "${source.build}", Kind: RefSource, Name: "build"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateReferences_InvalidModel(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Models: map[string]DeploymentModel{},
	}

	refs := []Reference{
		{Raw: "${models.missing.host}", Kind: RefModel, Name: "missing", Attribute: "host"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateReferences_InvalidAttributeForHostRef(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Models: map[string]DeploymentModel{
			"llm": {Image: "x", Endpoints: map[string]Endpoint{"http": {Port: 8080}}},
		},
	}

	// 3-part ref with port instead of host — invalid
	refs := []Reference{
		{Raw: "${models.llm.port}", Kind: RefModel, Name: "llm", Attribute: "port"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid 3-part attribute, got %d: %v", len(errs), errs)
	}
}

func TestValidateReferences_InvalidEndpointOnModel(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Models: map[string]DeploymentModel{
			"llm": {Image: "x", Endpoints: map[string]Endpoint{"http": {Port: 8080}}},
		},
	}

	// 4-part ref with non-existent endpoint
	refs := []Reference{
		{Raw: "${models.llm.grpc.port}", Kind: RefModel, Name: "llm", Endpoint: "grpc", Attribute: "port"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid endpoint, got %d: %v", len(errs), errs)
	}
}

func TestValidateReferences_InvalidVariable(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Variables: map[string]Variable{},
	}

	refs := []Reference{
		{Raw: "${variables.MISSING}", Kind: RefVariable, Name: "MISSING"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for missing variable, got %d", len(errs))
	}
}

func TestValidateReferences_InvalidSourceAttribute(t *testing.T) {
	ds := &AstroDeploymentSpec{}

	refs := []Reference{
		{Raw: "${source.invalid}", Kind: RefSource, Name: "invalid"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid source attr, got %d", len(errs))
	}
}

func TestValidateReferences_CredentialRefSelfHosted(t *testing.T) {
	// Credential refs should be valid for self-hosted provider-mode entries.
	ds := &AstroDeploymentSpec{
		Knowledge: map[string]DeploymentKnowledge{
			"pg": {Image: "pgvector:latest", Provider: "postgres", Endpoints: map[string]Endpoint{"http": {Port: 5432}}},
		},
	}

	refs := []Reference{
		{Raw: "${knowledge.pg.credentials.user}", Kind: RefKnowledge, Name: "pg", Endpoint: "credentials", Attribute: "user"},
		{Raw: "${knowledge.pg.credentials.password}", Kind: RefKnowledge, Name: "pg", Endpoint: "credentials", Attribute: "password"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) > 0 {
		t.Errorf("expected no errors for self-hosted credential refs, got: %v", errs)
	}
}

func TestValidateReferences_CredentialRefInvalidAttr(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Knowledge: map[string]DeploymentKnowledge{
			"pg": {Image: "pgvector:latest", Provider: "postgres", Endpoints: map[string]Endpoint{"http": {Port: 5432}}},
		},
	}

	refs := []Reference{
		{Raw: "${knowledge.pg.credentials.bogus}", Kind: RefKnowledge, Name: "pg", Endpoint: "credentials", Attribute: "bogus"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid credential attr, got %d: %v", len(errs), errs)
	}
}

func TestValidateReferences_CredentialRefNoProvider(t *testing.T) {
	// Container-mode entry with no provider — credentials ref should fail.
	ds := &AstroDeploymentSpec{
		Knowledge: map[string]DeploymentKnowledge{
			"custom": {Image: "mydb:latest", Endpoints: map[string]Endpoint{"http": {Port: 5432}}},
		},
	}

	refs := []Reference{
		{Raw: "${knowledge.custom.credentials.user}", Kind: RefKnowledge, Name: "custom", Endpoint: "credentials", Attribute: "user"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for container-mode credential ref, got %d: %v", len(errs), errs)
	}
}

func TestIsReference(t *testing.T) {
	if !IsReference("${models.llm.host}") {
		t.Error("expected true for reference string")
	}
	if IsReference("plain") {
		t.Error("expected false for plain string")
	}
}

func TestIsVariableReference(t *testing.T) {
	if !IsVariableReference("${variables.API_KEY}") {
		t.Error("expected true for variable reference")
	}
	if IsVariableReference("${models.llm.host}") {
		t.Error("expected false for model reference")
	}
	if IsVariableReference("plain") {
		t.Error("expected false for plain string")
	}
}
