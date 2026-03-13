package spec

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

//go:embed agent_card_integrations.json
var knownIntegrationsJSON []byte

// MaxAgentCardTags is the maximum number of tags allowed in an agent card.
const MaxAgentCardTags = 10

// MaxDescriptionLength is the maximum character length for an agent card description.
const MaxDescriptionLength = 200

// MaxCapabilityLength is the maximum character length for a single capability entry.
const MaxCapabilityLength = 100

// AgentCard represents the structured frontmatter metadata from an AGENT.md file.
type AgentCard struct {
	Description  string            `json:"description,omitempty" yaml:"description,omitempty"`
	Tags         []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Authors      []AgentCardAuthor `json:"authors,omitempty" yaml:"authors,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Integrations []string          `json:"-" yaml:"integrations,omitempty"`
}

// AgentCardAuthor represents an author entry in the agent card frontmatter.
type AgentCardAuthor struct {
	Name    string `json:"name" yaml:"name"`
	Account string `json:"account,omitempty" yaml:"account,omitempty"`
}

// KnownIntegration represents an entry in the known integrations registry.
type KnownIntegration struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
}

// ResolvedIntegration is an integration entry resolved against the known registry.
type ResolvedIntegration struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ParsedAgentCard is the result of parsing an AGENT.md file.
type ParsedAgentCard struct {
	AgentCard
	Body                 string                `json:"body"`
	ResolvedIntegrations []ResolvedIntegration `json:"integrations,omitempty"`
}

// ParseAgentCard parses raw AGENT.md content into structured metadata and a markdown body.
// It extracts YAML frontmatter (delimited by --- lines) and returns the remaining content as body.
func ParseAgentCard(content string) (*ParsedAgentCard, error) {
	result := &ParsedAgentCard{}

	if content == "" {
		return result, nil
	}

	// Frontmatter must start at the very beginning of the file with "---\n"
	if !strings.HasPrefix(content, "---\n") {
		result.Body = content
		return result, nil
	}

	// Find the closing "---" delimiter (search after the opening "---\n")
	rest := content[4:] // skip opening "---\n"

	var frontmatterYAML string
	var afterClosing string

	if strings.HasPrefix(rest, "---\n") || rest == "---" {
		// Empty frontmatter: ---\n---\n or ---\n---
		frontmatterYAML = ""
		afterClosing = strings.TrimPrefix(rest, "---\n")
		if afterClosing == "---" {
			afterClosing = ""
		}
	} else {
		closingIdx := strings.Index(rest, "\n---")
		if closingIdx == -1 {
			// No closing delimiter — treat the entire content as body (no valid frontmatter)
			result.Body = content
			return result, nil
		}
		frontmatterYAML = rest[:closingIdx]
		afterClosing = strings.TrimPrefix(rest[closingIdx+4:], "\n") // skip "\n---" and optional trailing newline
	}

	result.Body = afterClosing

	// Parse YAML frontmatter (empty frontmatter is fine — yields zero-value AgentCard)
	if strings.TrimSpace(frontmatterYAML) != "" {
		if err := yaml.Unmarshal([]byte(frontmatterYAML), &result.AgentCard); err != nil {
			return nil, fmt.Errorf("failed to parse agent card frontmatter: %w", err)
		}
	}

	// Validate limits
	if len(result.Tags) > MaxAgentCardTags {
		return nil, fmt.Errorf("agent card has %d tags, maximum is %d", len(result.Tags), MaxAgentCardTags)
	}

	// Normalize tags: lowercase, spaces→hyphens, strip invalid characters
	for i, tag := range result.Tags {
		result.Tags[i] = NormalizeTag(tag)
	}
	// Remove empty tags that resulted from normalization
	filtered := result.Tags[:0]
	for _, tag := range result.Tags {
		if tag != "" {
			filtered = append(filtered, tag)
		}
	}
	result.Tags = filtered

	// Truncate description to MaxDescriptionLength
	if len([]rune(result.Description)) > MaxDescriptionLength {
		runes := []rune(result.Description)
		result.Description = string(runes[:MaxDescriptionLength])
	}

	// Truncate each capability to MaxCapabilityLength
	for i, cap := range result.Capabilities {
		if len([]rune(cap)) > MaxCapabilityLength {
			runes := []rune(cap)
			result.Capabilities[i] = string(runes[:MaxCapabilityLength])
		}
	}

	// Normalize author accounts: lowercase, trim whitespace
	for i := range result.Authors {
		result.Authors[i].Account = strings.ToLower(strings.TrimSpace(result.Authors[i].Account))
	}

	// Resolve integrations against the known registry
	result.ResolvedIntegrations = MergeResolvedIntegrations(nil, result.Integrations)

	return result, nil
}

// NormalizeTag converts a tag string to a valid format: lowercase, spaces to hyphens,
// strip characters that aren't letters, numbers, or hyphens, collapse consecutive hyphens.
func NormalizeTag(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

// MergeResolvedIntegrations resolves additional integration strings and merges them
// into existing resolved integrations, deduplicating by ID.
func MergeResolvedIntegrations(existing []ResolvedIntegration, additional []string) []ResolvedIntegration {
	if len(additional) == 0 {
		return existing
	}
	seen := make(map[string]bool, len(existing))
	for _, ri := range existing {
		seen[ri.ID] = true
	}
	for _, s := range additional {
		known := ResolveIntegration(s)
		var ri ResolvedIntegration
		if known != nil {
			ri = ResolvedIntegration{ID: known.ID, Name: known.Name}
		} else {
			ri = ResolvedIntegration{ID: strings.ToLower(strings.TrimSpace(s)), Name: s}
		}
		if ri.ID != "" && !seen[ri.ID] {
			seen[ri.ID] = true
			existing = append(existing, ri)
		}
	}
	return existing
}

// ParseAgentCardFile reads and parses an AGENT.md file from the given path.
// If the file does not exist, it returns an empty ParsedAgentCard without error.
func ParseAgentCardFile(path string) (*ParsedAgentCard, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return &ParsedAgentCard{}, nil
		}
		return nil, fmt.Errorf("failed to read agent card: %w", err)
	}
	return ParseAgentCard(string(data))
}

// DeprecatedMetaFields checks raw spec YAML bytes for deprecated meta fields
// (description, tags) that have moved to AGENT.md frontmatter. Returns a list
// of human-readable deprecation messages for any found fields.
func DeprecatedMetaFields(specYAML []byte) []string {
	var raw map[string]any
	if err := yaml.Unmarshal(specYAML, &raw); err != nil {
		return nil
	}
	meta, ok := raw["meta"].(map[string]any)
	if !ok {
		return nil
	}
	var warnings []string
	if _, ok := meta["description"]; ok {
		warnings = append(warnings, "meta.description is deprecated in astropods.yml — move it to AGENT.md frontmatter")
	}
	if _, ok := meta["tags"]; ok {
		warnings = append(warnings, "meta.tags is deprecated in astropods.yml — move it to AGENT.md frontmatter")
	}
	return warnings
}

// ExtractLegacyMeta extracts deprecated description and tags from a raw spec map
// (as stored in spec_json). Used for backward-compatible display of existing agents
// that haven't migrated to AGENT.md.
func ExtractLegacyMeta(specMap map[string]any) (description string, tags []string) {
	meta, ok := specMap["meta"].(map[string]any)
	if !ok {
		return "", nil
	}
	if d, ok := meta["description"].(string); ok {
		description = d
	}
	if t, ok := meta["tags"].([]any); ok {
		for _, v := range t {
			if s, ok := v.(string); ok {
				tags = append(tags, s)
			}
		}
	}
	return description, tags
}

// knownIntegrations is the parsed registry, loaded once.
var knownIntegrations []KnownIntegration

// integrationLookup is a precomputed map for O(1) integration resolution.
// Keys are canonical IDs, lowercased display names, and lowercased aliases.
var integrationLookup map[string]*KnownIntegration

func init() {
	if err := json.Unmarshal(knownIntegrationsJSON, &knownIntegrations); err != nil {
		panic(fmt.Sprintf("failed to parse embedded agent_card_integrations.json: %v", err))
	}

	integrationLookup = make(map[string]*KnownIntegration, len(knownIntegrations)*3)
	for i := range knownIntegrations {
		ki := &knownIntegrations[i]
		integrationLookup[ki.ID] = ki
		integrationLookup[strings.ToLower(ki.Name)] = ki
		for _, alias := range ki.Aliases {
			integrationLookup[strings.ToLower(alias)] = ki
		}
	}
}

// KnownIntegrations returns the full list of known integrations from the embedded registry.
func KnownIntegrations() []KnownIntegration {
	return knownIntegrations
}

// ResolveIntegration matches an integration string against the known integrations registry.
// Returns nil if no match is found (unknown integration).
//
// Matching rules:
//  1. Normalize input: lowercase, trim whitespace.
//  2. Look up in the precomputed map (covers id, name, and alias matches).
//  3. No match → return nil.
func ResolveIntegration(name string) *KnownIntegration {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return nil
	}
	return integrationLookup[normalized]
}
