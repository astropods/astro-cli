package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentCard(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(*testing.T, *ParsedAgentCard)
	}{
		{
			name: "full frontmatter and body",
			content: `---
description: "GitHub Issue Analyzer"
tags:
  - analytics
  - knowledge-graph
authors:
  - name: Jane Doe
    account: janedoe
  - name: Bob Smith
capabilities:
  - Analyzes GitHub issues
  - Builds a knowledge graph
integrations:
  - Jira
  - My Custom Webhook
---
# GitHub Issue Analyzer

This agent ingests GitHub issues.
`,
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "GitHub Issue Analyzer" {
					t.Errorf("Description = %q, want %q", p.Description, "GitHub Issue Analyzer")
				}
				if len(p.Tags) != 2 || p.Tags[0] != "analytics" || p.Tags[1] != "knowledge-graph" {
					t.Errorf("Tags = %v, want [analytics knowledge-graph]", p.Tags)
				}
				if len(p.Authors) != 2 {
					t.Fatalf("len(Authors) = %d, want 2", len(p.Authors))
				}
				if p.Authors[0].Name != "Jane Doe" || p.Authors[0].Account != "janedoe" {
					t.Errorf("Authors[0] = %+v", p.Authors[0])
				}
				if p.Authors[1].Name != "Bob Smith" || p.Authors[1].Account != "" {
					t.Errorf("Authors[1] = %+v", p.Authors[1])
				}
				if len(p.Capabilities) != 2 {
					t.Fatalf("len(Capabilities) = %d, want 2", len(p.Capabilities))
				}
				if len(p.Integrations) != 2 || p.Integrations[0] != "Jira" || p.Integrations[1] != "My Custom Webhook" {
					t.Errorf("Integrations = %v", p.Integrations)
				}
				if p.Body != "# GitHub Issue Analyzer\n\nThis agent ingests GitHub issues.\n" {
					t.Errorf("Body = %q", p.Body)
				}
			},
		},
		{
			name:    "no frontmatter - plain markdown",
			content: "# My Agent\n\nSome description here.\n",
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "" {
					t.Errorf("Description = %q, want empty", p.Description)
				}
				if p.Body != "# My Agent\n\nSome description here.\n" {
					t.Errorf("Body = %q", p.Body)
				}
			},
		},
		{
			name:    "empty frontmatter",
			content: "---\n---\nSome body content.\n",
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "" {
					t.Errorf("Description = %q, want empty", p.Description)
				}
				if p.Body != "Some body content.\n" {
					t.Errorf("Body = %q", p.Body)
				}
			},
		},
		{
			name: "frontmatter only - no body",
			content: `---
description: "Just metadata"
tags:
  - test
---
`,
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "Just metadata" {
					t.Errorf("Description = %q, want %q", p.Description, "Just metadata")
				}
				if p.Body != "" {
					t.Errorf("Body = %q, want empty", p.Body)
				}
			},
		},
		{
			name:    "empty file",
			content: "",
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "" {
					t.Errorf("Description = %q, want empty", p.Description)
				}
				if p.Body != "" {
					t.Errorf("Body = %q, want empty", p.Body)
				}
			},
		},
		{
			name: "unknown frontmatter fields ignored",
			content: `---
description: "Known field"
unknown_field: some value
another_unknown:
  nested: true
---
Body here.
`,
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "Known field" {
					t.Errorf("Description = %q, want %q", p.Description, "Known field")
				}
				if p.Body != "Body here.\n" {
					t.Errorf("Body = %q", p.Body)
				}
			},
		},
		{
			name: "malformed YAML in frontmatter",
			content: `---
description: [invalid yaml
---
Body.
`,
			wantErr: true,
		},
		{
			name: "body containing horizontal rules not confused for frontmatter",
			content: `---
description: "My Agent"
---
# Section 1

Some text.

---

# Section 2

More text.

---

Final section.
`,
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "My Agent" {
					t.Errorf("Description = %q, want %q", p.Description, "My Agent")
				}
				expected := "# Section 1\n\nSome text.\n\n---\n\n# Section 2\n\nMore text.\n\n---\n\nFinal section.\n"
				if p.Body != expected {
					t.Errorf("Body = %q, want %q", p.Body, expected)
				}
			},
		},
		{
			name: "frontmatter with no closing delimiter",
			content: `---
description: "Unclosed"
`,
			check: func(t *testing.T, p *ParsedAgentCard) {
				// No closing --- means no valid frontmatter; entire content is body
				if p.Description != "" {
					t.Errorf("Description = %q, want empty", p.Description)
				}
				if p.Body != "---\ndescription: \"Unclosed\"\n" {
					t.Errorf("Body = %q", p.Body)
				}
			},
		},
		{
			name: "frontmatter only no trailing newline",
			content: "---\ndescription: \"test\"\n---",
			check: func(t *testing.T, p *ParsedAgentCard) {
				if p.Description != "test" {
					t.Errorf("Description = %q, want %q", p.Description, "test")
				}
				if p.Body != "" {
					t.Errorf("Body = %q, want empty", p.Body)
				}
			},
		},
		{
			name: "too many tags",
			content: `---
tags:
  - one
  - two
  - three
  - four
  - five
  - six
  - seven
  - eight
  - nine
  - ten
  - eleven
---
Body.
`,
			wantErr: true,
		},
		{
			name: "exactly 10 tags is allowed",
			content: `---
tags: [a, b, c, d, e, f, g, h, i, j]
---
Body.
`,
			check: func(t *testing.T, p *ParsedAgentCard) {
				if len(p.Tags) != 10 {
					t.Errorf("len(Tags) = %d, want 10", len(p.Tags))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAgentCard(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAgentCard() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

func TestParseAgentCardFile(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "AGENT.md")
		content := "---\ndescription: \"File test\"\n---\n# Hello\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := ParseAgentCardFile(path)
		if err != nil {
			t.Fatalf("ParseAgentCardFile() error = %v", err)
		}
		if result.Description != "File test" {
			t.Errorf("Description = %q, want %q", result.Description, "File test")
		}
		if result.Body != "# Hello\n" {
			t.Errorf("Body = %q", result.Body)
		}
	})

	t.Run("nonexistent file returns empty without error", func(t *testing.T) {
		result, err := ParseAgentCardFile("/nonexistent/AGENT.md")
		if err != nil {
			t.Fatalf("ParseAgentCardFile() error = %v, want nil", err)
		}
		if result.Description != "" || result.Body != "" {
			t.Errorf("expected empty result, got Description=%q Body=%q", result.Description, result.Body)
		}
	})
}

func TestResolveIntegration(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantID string // empty string means expect nil
	}{
		{"match by id", "slack", "slack"},
		{"match by id with whitespace", "  github  ", "github"},
		{"match by display name", "GitHub", "github"},
		{"match by display name case insensitive", "SLACK", "slack"},
		{"match by display name - Google Drive", "Google Drive", "google-drive"},
		{"match by id - google-drive", "google-drive", "google-drive"},
		{"match by display name - Microsoft Teams", "Microsoft Teams", "microsoft-teams"},
		{"match by alias", "gh", "github"},
		{"match by alias case insensitive", "GH", "github"},
		{"match by alias - slackbot", "slackbot", "slack"},
		{"match by alias - ms-teams", "ms-teams", "microsoft-teams"},
		{"match by alias - gdrive", "gdrive", "google-drive"},
		{"unknown integration", "My Custom API", ""},
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveIntegration(tt.input)
			if tt.wantID == "" {
				if result != nil {
					t.Errorf("ResolveIntegration(%q) = %+v, want nil", tt.input, result)
				}
			} else {
				if result == nil {
					t.Fatalf("ResolveIntegration(%q) = nil, want id=%q", tt.input, tt.wantID)
				}
				if result.ID != tt.wantID {
					t.Errorf("ResolveIntegration(%q).ID = %q, want %q", tt.input, result.ID, tt.wantID)
				}
			}
		})
	}
}

func TestKnownIntegrations(t *testing.T) {
	integrations := KnownIntegrations()
	if len(integrations) == 0 {
		t.Fatal("KnownIntegrations() returned empty list")
	}
	// Verify the first entry matches what we expect from the JSON
	if integrations[0].ID != "slack" || integrations[0].Name != "Slack" {
		t.Errorf("first integration = %+v, want {ID:slack Name:Slack}", integrations[0])
	}
}

func TestNormalizeTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"analytics", "analytics"},
		{"Knowledge Graph", "knowledge-graph"},
		{"UPPERCASE", "uppercase"},
		{"  spaces  ", "spaces"},
		{"special!@#chars", "specialchars"},
		{"multiple   spaces", "multiple-spaces"},
		{"already-hyphenated", "already-hyphenated"},
		{"---leading-trailing---", "leading-trailing"},
		{"café", "café"},
		{"data & analytics", "data-analytics"},
		{"", ""},
		{"---", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeTag(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseAgentCard_TagNormalization(t *testing.T) {
	content := `---
tags:
  - Analytics
  - Knowledge Graph
  - "data & ML"
---
Body.
`
	result, err := ParseAgentCard(content)
	if err != nil {
		t.Fatalf("ParseAgentCard() error = %v", err)
	}
	want := []string{"analytics", "knowledge-graph", "data-ml"}
	if len(result.Tags) != len(want) {
		t.Fatalf("len(Tags) = %d, want %d: %v", len(result.Tags), len(want), result.Tags)
	}
	for i, tag := range result.Tags {
		if tag != want[i] {
			t.Errorf("Tags[%d] = %q, want %q", i, tag, want[i])
		}
	}
}

func TestParseAgentCard_DescriptionTruncation(t *testing.T) {
	// 210-char description should be truncated to 200
	long := strings.Repeat("a", 210)
	content := "---\ndescription: \"" + long + "\"\n---\n"
	result, err := ParseAgentCard(content)
	if err != nil {
		t.Fatalf("ParseAgentCard() error = %v", err)
	}
	if len([]rune(result.Description)) != MaxDescriptionLength {
		t.Errorf("Description length = %d, want %d", len([]rune(result.Description)), MaxDescriptionLength)
	}
}

func TestParseAgentCard_CapabilityTruncation(t *testing.T) {
	long := strings.Repeat("b", 120)
	content := "---\ncapabilities:\n  - \"" + long + "\"\n  - short\n---\n"
	result, err := ParseAgentCard(content)
	if err != nil {
		t.Fatalf("ParseAgentCard() error = %v", err)
	}
	if len(result.Capabilities) != 2 {
		t.Fatalf("len(Capabilities) = %d, want 2", len(result.Capabilities))
	}
	if len([]rune(result.Capabilities[0])) != MaxCapabilityLength {
		t.Errorf("Capabilities[0] length = %d, want %d", len([]rune(result.Capabilities[0])), MaxCapabilityLength)
	}
	if result.Capabilities[1] != "short" {
		t.Errorf("Capabilities[1] = %q, want %q", result.Capabilities[1], "short")
	}
}

func TestParseAgentCard_AuthorAccountNormalization(t *testing.T) {
	content := `---
authors:
  - name: Jane Doe
    account: "  JaneDoe  "
  - name: Bob Smith
---
`
	result, err := ParseAgentCard(content)
	if err != nil {
		t.Fatalf("ParseAgentCard() error = %v", err)
	}
	if result.Authors[0].Account != "janedoe" {
		t.Errorf("Authors[0].Account = %q, want %q", result.Authors[0].Account, "janedoe")
	}
	if result.Authors[0].Name != "Jane Doe" {
		t.Errorf("Authors[0].Name = %q, want %q", result.Authors[0].Name, "Jane Doe")
	}
}

func TestMergeResolvedIntegrations(t *testing.T) {
	existing := []ResolvedIntegration{
		{ID: "slack", Name: "Slack"},
	}

	result := MergeResolvedIntegrations(existing, []string{"GitHub", "Slack", "My Custom API"})

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3: %v", len(result), result)
	}
	// slack was already present, should not be duplicated
	if result[0].ID != "slack" {
		t.Errorf("result[0].ID = %q, want %q", result[0].ID, "slack")
	}
	if result[1].ID != "github" {
		t.Errorf("result[1].ID = %q, want %q", result[1].ID, "github")
	}
	// Unknown integration should use raw string as name
	if result[2].ID != "my custom api" || result[2].Name != "My Custom API" {
		t.Errorf("result[2] = %+v, want {ID:my custom api Name:My Custom API}", result[2])
	}
}

func TestMergeResolvedIntegrations_NilExisting(t *testing.T) {
	result := MergeResolvedIntegrations(nil, []string{"Slack"})
	if len(result) != 1 || result[0].ID != "slack" {
		t.Errorf("result = %v, want [{slack Slack}]", result)
	}
}

func TestMergeResolvedIntegrations_EmptyAdditional(t *testing.T) {
	existing := []ResolvedIntegration{{ID: "slack", Name: "Slack"}}
	result := MergeResolvedIntegrations(existing, nil)
	if len(result) != 1 {
		t.Errorf("len(result) = %d, want 1", len(result))
	}
}
