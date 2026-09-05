package cmd

import (
	spec "github.com/astropods/astro-spec"
)

// agentCardWarnings reports what the AGENT.md beside the spec leaves unset,
// invalid, or truncated. An empty result means the card is complete.
func agentCardWarnings(workingDir string) []string {
	path := findAgentReadme(workingDir)
	if path == "" {
		return []string{msgAgentCardMissing()}
	}

	card, err := spec.ParseAgentCardFile(path)
	if err != nil {
		return nil
	}

	var warnings []string
	for _, w := range card.Warnings {
		warnings = append(warnings, msgAgentCardParseWarning(w))
	}
	for _, field := range spec.MissingAttribution(&card.AgentCard) {
		switch field {
		case spec.AttributionFieldAuthors:
			warnings = append(warnings, msgAgentCardMissingAuthors())
		case spec.AttributionFieldRepository:
			warnings = append(warnings, msgAgentCardMissingRepository())
		}
	}
	return warnings
}
