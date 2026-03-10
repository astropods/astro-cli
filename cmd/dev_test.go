package cmd

import (
	"slices"
	"testing"
)

func TestComposeBuildArgs(t *testing.T) {
	tests := []struct {
		name      string
		rebuild   bool
		noPull    bool
		wantPull  bool
		wantCache bool
	}{
		{
			name:      "normal mode pulls base images",
			rebuild:   false,
			noPull:    false,
			wantPull:  true,
			wantCache: true,
		},
		{
			name:      "local mode skips pull",
			rebuild:   false,
			noPull:    true,
			wantPull:  false,
			wantCache: true,
		},
		{
			name:      "rebuild disables cache and pulls",
			rebuild:   true,
			noPull:    false,
			wantPull:  true,
			wantCache: false,
		},
		{
			name:      "rebuild + no-pull disables both",
			rebuild:   true,
			noPull:    true,
			wantPull:  false,
			wantCache: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := composeBuildArgs("test-compose.yml", tt.rebuild, tt.noPull)

			hasPull := slices.Contains(args, "--pull")
			if hasPull != tt.wantPull {
				t.Errorf("--pull present = %v, want %v (args: %v)", hasPull, tt.wantPull, args)
			}

			hasNoCache := slices.Contains(args, "--no-cache")
			if hasNoCache == tt.wantCache {
				t.Errorf("--no-cache present = %v, want %v (args: %v)", hasNoCache, !tt.wantCache, args)
			}

			if hasPull && slices.Contains(args, "--pull=never") {
				t.Error("--pull=never must not appear; --pull is a boolean flag on docker compose build")
			}
		})
	}
}
