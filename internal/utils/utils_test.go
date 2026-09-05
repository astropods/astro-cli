package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEnvFile(t *testing.T) {
	tests := []struct {
		name     string
		envFile  string
		setup    func(t *testing.T, dir string)
		wantErr  bool
		wantVars map[string]string
	}{
		{
			name:    "reads a real env file",
			envFile: DefaultEnvFile,
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultEnvFile),
					[]byte("FOO=bar\n# comment\nBAZ=qux\n"), 0o600))
			},
			wantVars: map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:    "missing file is not an error",
			envFile: DefaultEnvFile,
			setup:   func(*testing.T, string) {},
		},
		{
			// The regression this function exists to prevent. An empty envFile
			// makes filepath.Join collapse to workingDir; Stat then succeeds
			// because the directory exists, so godotenv was handed a directory
			// and callers died with "read <project>: is a directory". A command
			// reading an unregistered flag gets "" for free, so this is
			// reachable by accident — it is how `project trigger` broke for
			// every ingestion job.
			name:    "empty envFile does not read the working directory",
			envFile: "",
			setup:   func(*testing.T, string) {},
		},
		{
			name:    "a directory at the env path is not an error",
			envFile: DefaultEnvFile,
			setup: func(t *testing.T, dir string) {
				require.NoError(t, os.Mkdir(filepath.Join(dir, DefaultEnvFile), 0o700))
			},
		},
		{
			name:    "malformed file still reports an error",
			envFile: DefaultEnvFile,
			setup: func(t *testing.T, dir string) {
				// Unparseable by godotenv: a bare key with no separator.
				require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultEnvFile),
					[]byte("this is not an env file\n"), 0o600))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			vars, err := LoadEnvFile(dir, tt.envFile)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantVars == nil {
				assert.Nil(t, vars)
				return
			}
			for k, want := range tt.wantVars {
				assert.Equal(t, want, vars[k])
			}
		})
	}
}
