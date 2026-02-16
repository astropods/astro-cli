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
		wantAttr string
	}{
		{"model host", "${models.local_llm.host}", 1, RefModel, "local_llm", "host"},
		{"model port", "${models.local_llm.port}", 1, RefModel, "local_llm", "port"},
		{"model url", "${models.local_llm.url}", 1, RefModel, "local_llm", "url"},
		{"knowledge host", "${knowledge.docs.host}", 1, RefKnowledge, "docs", "host"},
		{"tool url", "${tools.search.url}", 1, RefTool, "search", "url"},
		{"credential", "${credentials.API_KEY}", 1, RefCredential, "API_KEY", ""},
		{"source name", "${source.name}", 1, RefSource, "name", ""},
		{"source build", "${source.build}", 1, RefSource, "build", ""},
		{"no refs", "plain string", 0, "", "", ""},
		{"partial ref ignored", "${invalid}", 0, "", "", ""},
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
			if ref.Attribute != tt.wantAttr {
				t.Errorf("attribute: expected %s, got %s", tt.wantAttr, ref.Attribute)
			}
		})
	}
}

func TestParseReferences_MultipleInOneString(t *testing.T) {
	refs := ParseReferences("http://${models.llm.host}:${models.llm.port}")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].Attribute != "host" || refs[1].Attribute != "port" {
		t.Errorf("expected host and port attrs, got %s and %s", refs[0].Attribute, refs[1].Attribute)
	}
}

func TestExtractAllReferences(t *testing.T) {
	env := map[string]string{
		"LLM_URL":     "${models.llm.url}",
		"QDRANT_HOST": "${knowledge.docs.host}",
		"API_KEY":     "${credentials.ANTHROPIC_API_KEY}",
		"STATIC":      "no_refs_here",
	}

	refs := ExtractAllReferences(env)
	if len(refs) != 3 {
		t.Errorf("expected 3 refs from env map, got %d", len(refs))
	}
}

func TestValidateReferences_Valid(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Models:    map[string]DeploymentModel{"llm": {Image: "x", Port: 8080}},
		Knowledge: map[string]DeploymentKnowledge{"docs": {Image: "x", Port: 6333}},
		Tools:     map[string]DeploymentTool{"search": {Image: "x", Port: 3000}},
		Credentials: map[string]DeploymentCredential{
			"API_KEY": {Description: "key"},
		},
	}

	refs := []Reference{
		{Raw: "${models.llm.host}", Kind: RefModel, Name: "llm", Attribute: "host"},
		{Raw: "${models.llm.port}", Kind: RefModel, Name: "llm", Attribute: "port"},
		{Raw: "${models.llm.url}", Kind: RefModel, Name: "llm", Attribute: "url"},
		{Raw: "${knowledge.docs.host}", Kind: RefKnowledge, Name: "docs", Attribute: "host"},
		{Raw: "${tools.search.url}", Kind: RefTool, Name: "search", Attribute: "url"},
		{Raw: "${credentials.API_KEY}", Kind: RefCredential, Name: "API_KEY"},
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

func TestValidateReferences_InvalidAttribute(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Models: map[string]DeploymentModel{"llm": {Image: "x", Port: 8080}},
	}

	refs := []Reference{
		{Raw: "${models.llm.invalid}", Kind: RefModel, Name: "llm", Attribute: "invalid"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for invalid attribute, got %d: %v", len(errs), errs)
	}
}

func TestValidateReferences_InvalidCredential(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Credentials: map[string]DeploymentCredential{},
	}

	refs := []Reference{
		{Raw: "${credentials.MISSING}", Kind: RefCredential, Name: "MISSING"},
	}

	errs := ValidateReferences(refs, ds)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for missing credential, got %d", len(errs))
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

func TestIsReference(t *testing.T) {
	if !IsReference("${models.llm.host}") {
		t.Error("expected true for reference string")
	}
	if IsReference("plain") {
		t.Error("expected false for plain string")
	}
}

func TestIsCredentialReference(t *testing.T) {
	if !IsCredentialReference("${credentials.API_KEY}") {
		t.Error("expected true for credential reference")
	}
	if IsCredentialReference("${models.llm.host}") {
		t.Error("expected false for model reference")
	}
	if IsCredentialReference("plain") {
		t.Error("expected false for plain string")
	}
}
