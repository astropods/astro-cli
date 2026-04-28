package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParentCmdsRejectUnknownSubcommands(t *testing.T) {
	cases := [][]string{
		{"bp", "validate"},       // validate was moved to `spec`; bp now silently accepts it
		{"spec", "foo-not-real"}, // new parent in this PR has the same default behavior
	}

	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			rootCmd.SetOut(&stdout)
			rootCmd.SetErr(&stderr)
			rootCmd.SetArgs(args)
			t.Cleanup(func() {
				rootCmd.SetOut(nil)
				rootCmd.SetErr(nil)
				rootCmd.SetArgs(nil)
			})

			err := rootCmd.Execute()

			require.Error(t, err,
				"%q exits 0 today; should error so `set -e`/CI catches typos and stale references",
				strings.Join(args, " "))
			assert.Contains(t, err.Error(), "unknown command")
		})
	}
}
