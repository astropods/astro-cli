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

	tests := []struct {
		name    string
		envFile string
		want    map[string]string
	}{
		{name: "reads the named file", envFile: DefaultEnvFile, want: map[string]string{"A": "1"}},
		{name: "a missing file is not an error", envFile: ".env.absent"},
		{name: "an empty name is not read as a file", envFile: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadEnvFile(dir, tt.envFile)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
