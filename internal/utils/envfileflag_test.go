package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvFileFlagSet(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prod.env"), []byte("A=1\n"), 0o600))
	t.Chdir(dir)

	absFile := filepath.Join(t.TempDir(), "abs.env")
	require.NoError(t, os.WriteFile(absFile, []byte("B=2\n"), 0o600))

	tests := []struct {
		name      string
		value     string
		wantErrIs error
	}{
		{name: "a relative path resolves against the working directory", value: "prod.env"},
		{name: "an absolute path is taken as given", value: absFile},
		{name: "a missing file is rejected as the flag is parsed", value: "typo.env", wantErrIs: ErrEnvFileNotFound},
		{name: "a directory is not a file", value: ".", wantErrIs: ErrEnvFileNotFound},
		{name: "an empty value names nothing", value: "", wantErrIs: ErrEnvFileNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewEnvFileFlag(DefaultEnvFile)
			err := f.Set(tt.value)

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Equal(t, DefaultEnvFile, f.String(),
					"a rejected value must not replace the default")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.value, f.String(), "the accepted value is what the command reads")
		})
	}
}

func TestEnvFileFlag_DefaultIsNeverValidated(t *testing.T) {
	t.Chdir(t.TempDir()) // no .env here

	f := NewEnvFileFlag(DefaultEnvFile)

	assert.Equal(t, DefaultEnvFile, f.String(),
		"a default the user never asked for must not have to exist")
}

func TestEnvFileFlag_TypeIsStringSoGetStringKeepsWorking(t *testing.T) {
	// pflag's GetString rejects any Value whose Type is not "string", which
	// would make every flagString read of these flags return "".
	assert.Equal(t, "string", NewEnvFileFlag("").Type())
}
