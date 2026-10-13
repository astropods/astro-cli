package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/config"
)

func TestParseDeployVarsFromLocal(t *testing.T) {
	tests := []struct {
		name    string
		stored  map[string]string
		envFile string
		args    []string
		want    map[string]deployVarInput
		wantErr error
	}{
		{
			name:   "saved values become plain variables",
			stored: map[string]string{"OPENAI_API_KEY": "sk-local", "MODEL": "gpt"},
			args:   []string{"--from-local"},
			want: map[string]deployVarInput{
				"OPENAI_API_KEY": {Value: "sk-local"},
				"MODEL":          {Value: "gpt"},
			},
		},
		{
			name:   "--var wins over a saved value",
			stored: map[string]string{"MODEL": "gpt", "REGION": "us"},
			args:   []string{"--from-local", "--var", "MODEL=claude"},
			want: map[string]deployVarInput{
				"MODEL":  {Value: "claude"},
				"REGION": {Value: "us"},
			},
		},
		{
			name:    "--vars-file wins over a saved value",
			stored:  map[string]string{"MODEL": "gpt", "REGION": "us"},
			envFile: "MODEL=from-file\n",
			args:    []string{"--from-local", "--vars-file", ".env"},
			want: map[string]deployVarInput{
				"MODEL":  {Value: "from-file"},
				"REGION": {Value: "us"},
			},
		},
		{
			name:    "nothing saved for the directory is an error, not an empty deploy",
			stored:  nil,
			args:    []string{"--from-local"},
			wantErr: errNoLocalProjectVars(),
		},
		{
			name:   "without the flag, saved values are not sent",
			stored: map[string]string{"MODEL": "gpt"},
			args:   []string{"--var", "REGION=us"},
			want:   map[string]deployVarInput{"REGION": {Value: "us"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			workingDir := t.TempDir()
			t.Chdir(workingDir)
			if tt.envFile != "" {
				require.NoError(t, os.WriteFile(".env", []byte(tt.envFile), 0o600))
			}
			if tt.stored != nil {
				require.NoError(t, config.SetProjectVars(buildinfo.BinaryName, workingDir, "ag", tt.stored))
			}

			cmd := &cobra.Command{Use: "deploy"}
			registerDeployCommonFlags(cmd)
			require.NoError(t, cmd.ParseFlags(tt.args))

			vars, err := parseDeployVarsFromCmd(cmd)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, vars)
		})
	}
}
