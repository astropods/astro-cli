package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultEnvFile), []byte("A=1\n"), 0o600))

	elsewhere := t.TempDir()
	absFile := filepath.Join(elsewhere, "prod.env")
	require.NoError(t, os.WriteFile(absFile, []byte("B=2\n"), 0o600))

	tests := []struct {
		name      string
		envFile   string
		want      map[string]string
		wantErrIs error
	}{
		{
			name:    "reads the named file",
			envFile: DefaultEnvFile,
			want:    map[string]string{"A": "1"},
		},
		{
			name:    "an absolute path is read from where it points, not rejoined onto dir",
			envFile: absFile,
			want:    map[string]string{"B": "2"},
		},
		{
			name:      "a named file that is missing is an error, so a typo cannot pass as defaults",
			envFile:   ".env.absent",
			wantErrIs: ErrEnvFileNotFound,
		},
		{
			name:      "a directory is not a file",
			envFile:   ".",
			wantErrIs: ErrEnvFileNotFound,
		},
		{
			name:    "an empty name requests no file at all",
			envFile: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadEnvFile(dir, tt.envFile)
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Nil(t, got, "no vars come back with an error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnvFilePath(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		envFile string
		want    string
	}{
		{
			name: "a relative name resolves against dir",
			dir:  "/proj", envFile: "env/prod.env", want: "/proj/env/prod.env",
		},
		{
			// filepath.Join("/proj", "/abs/prod.env") yields /proj/abs/prod.env,
			// reading a file the user never named.
			name: "an absolute name is left alone",
			dir:  "/proj", envFile: "/abs/prod.env", want: "/abs/prod.env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EnvFilePath(tt.dir, tt.envFile))
		})
	}
}

func TestReadEnvFile_ReportsAnUnparseableFileDistinctlyFromAMissingOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.env")
	require.NoError(t, os.WriteFile(path, []byte("A=1\nnot an env line\n"), 0o600))

	_, err := ReadEnvFile(path)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrEnvFileNotFound,
		"a parse failure must not read as an absent file, which callers are allowed to ignore")
}
