package cmd

import (
	"slices"
	"testing"
)

func TestComposeBuildArgs(t *testing.T) {
	tests := []struct {
		name           string
		rebuild        bool
		noPull         bool
		wantPull       bool
		wantPullFalse  bool
		wantNoCache    bool
	}{
		{
			name:          "normal mode pulls base images",
			rebuild:       false,
			noPull:        false,
			wantPull:      true,
			wantPullFalse: false,
			wantNoCache:   false,
		},
		{
			name:          "local mode explicitly disables pull",
			rebuild:       false,
			noPull:        true,
			wantPull:      false,
			wantPullFalse: true,
			wantNoCache:   false,
		},
		{
			name:          "rebuild disables cache and pulls",
			rebuild:       true,
			noPull:        false,
			wantPull:      true,
			wantPullFalse: false,
			wantNoCache:   true,
		},
		{
			name:          "rebuild + no-pull disables cache and pull",
			rebuild:       true,
			noPull:        true,
			wantPull:      false,
			wantPullFalse: true,
			wantNoCache:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := composeBuildArgs("test-compose.yml", tt.rebuild, tt.noPull)

			hasPull := slices.Contains(args, "--pull")
			if hasPull != tt.wantPull {
				t.Errorf("--pull present = %v, want %v (args: %v)", hasPull, tt.wantPull, args)
			}

			hasPullFalse := slices.Contains(args, "--pull=false")
			if hasPullFalse != tt.wantPullFalse {
				t.Errorf("--pull=false present = %v, want %v (args: %v)", hasPullFalse, tt.wantPullFalse, args)
			}

			if hasPull && hasPullFalse {
				t.Errorf("--pull and --pull=false are mutually exclusive (args: %v)", args)
			}

			hasNoCache := slices.Contains(args, "--no-cache")
			if hasNoCache != tt.wantNoCache {
				t.Errorf("--no-cache present = %v, want %v (args: %v)", hasNoCache, tt.wantNoCache, args)
			}
		})
	}
}
