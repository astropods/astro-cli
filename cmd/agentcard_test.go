package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeAgentCard writes an AGENT.md in a new temp dir and returns the dir.
func writeAgentCard(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(content), 0o600))
	return dir
}

func TestAgentCardWarnings(t *testing.T) {
	tests := []struct {
		name string
		card string
		want []string
	}{
		{
			name: "complete card warns about nothing",
			card: "---\ndescription: Finds things\nauthors:\n  - name: Jane Doe\n    account: janedoe\nrepository: github:astropods/scout\n---\nBody.\n",
			want: nil,
		},
		{
			name: "scaffolded placeholders warn about both fields",
			card: "---\ndescription: Finds things\nauthors: []\n---\nBody.\n",
			want: []string{msgAgentCardMissingAuthors(), msgAgentCardMissingRepository()},
		},
		{
			name: "author without a repository warns about the repository",
			card: "---\nauthors:\n  - account: janedoe\n---\nBody.\n",
			want: []string{msgAgentCardMissingRepository()},
		},
		{
			name: "parser warnings come through with the attribution warnings",
			card: "---\nauthors: janedoe\nrepository: github:astropods/scout\n---\nBody.\n",
			want: []string{msgAgentCardParseWarning("authors: expected a list, dropped"), msgAgentCardMissingAuthors()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, agentCardWarnings(writeAgentCard(t, tt.card)))
		})
	}
}

func TestAgentCardWarnings_NoCard(t *testing.T) {
	assert.Equal(t, []string{msgAgentCardMissing()}, agentCardWarnings(t.TempDir()))
}
