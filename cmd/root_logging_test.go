package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommandSetsDependencyLogLevelFromVerboseFlag(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantLevel logrus.Level
		wantWarn  bool
	}{
		{name: "without verbose", args: []string{"knowledge", "status"}, wantLevel: logrus.ErrorLevel, wantWarn: false},
		{name: "with verbose", args: []string{"knowledge", "status", "--verbose"}, wantLevel: logrus.InfoLevel, wantWarn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			prevLevel := logrus.GetLevel()
			logrus.SetLevel(logrus.DebugLevel)
			logrus.SetOutput(&logs)
			var stdout, stderr bytes.Buffer
			rootCmd.SetOut(&stdout)
			rootCmd.SetErr(&stderr)
			rootCmd.SetArgs(tc.args)
			t.Cleanup(func() {
				rootCmd.SetOut(nil)
				rootCmd.SetErr(nil)
				rootCmd.SetArgs(nil)
				require.NoError(t, rootCmd.PersistentFlags().Set("verbose", "false"))
				logrus.SetLevel(prevLevel)
				logrus.SetOutput(os.Stderr)
			})

			err := rootCmd.Execute()
			require.Error(t, err, "knowledge status without a name fails argument validation after the init hooks run")

			assert.Equal(t, tc.wantLevel, logrus.GetLevel())

			logrus.Warn("compose teardown warning")
			assert.Equal(t, tc.wantWarn, logs.Len() > 0,
				"compose SDK warnings reach the terminal only with --verbose; got %q", logs.String())

			logs.Reset()
			logrus.Error("compose failure")
			assert.Contains(t, logs.String(), "compose failure", "dependency errors still surface without --verbose")
		})
	}
}
