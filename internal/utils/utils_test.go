package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEnvPath(t *testing.T) {
	workingDir := filepath.Join(string(filepath.Separator), "proj")
	absolute := filepath.Join(string(filepath.Separator), "tmp", "run.env")

	tests := []struct {
		name    string
		envFile string
		want    string
	}{
		{name: "an absolute path resolves to itself", envFile: absolute, want: absolute},
		{name: "a relative path resolves against the working directory", envFile: filepath.Join("cfg", "run.env"), want: filepath.Join(workingDir, "cfg", "run.env")},
		{name: "the default name resolves inside the working directory", envFile: DefaultEnvFile, want: filepath.Join(workingDir, DefaultEnvFile)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveEnvPath(workingDir, tt.envFile))
		})
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultEnvFile), []byte("A=1\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o700))

	outside := t.TempDir()
	absFile := filepath.Join(outside, "run.env")
	require.NoError(t, os.WriteFile(absFile, []byte("B=2\n"), 0o600))

	tests := []struct {
		name     string
		envFile  string
		required bool
		want     map[string]string
		wantErr  error
	}{
		{name: "reads the named file", envFile: DefaultEnvFile, want: map[string]string{"A": "1"}},
		{name: "reads an absolute path outside the working directory", envFile: absFile, required: true, want: map[string]string{"B": "2"}},
		{name: "a missing optional file is not an error", envFile: ".env.absent"},
		{name: "an empty optional name is not read as a file", envFile: ""},
		{name: "a missing required file is an error", envFile: ".env.absent", required: true, wantErr: ErrEnvFileNotFound},
		{name: "a missing required absolute file is an error", envFile: filepath.Join(outside, "absent.env"), required: true, wantErr: ErrEnvFileNotFound},
		{name: "a required directory is an error", envFile: "subdir", required: true, wantErr: ErrEnvFileNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadEnvFile(dir, tt.envFile, tt.required)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
