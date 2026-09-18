package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLayerEnv(t *testing.T) {
	store := EnvLayer{Name: "store", Vars: map[string]string{"K": "from-store", "ONLY_STORE": "s"}}
	file := EnvLayer{Name: "file", Vars: map[string]string{"K": "from-file", "ONLY_FILE": "f"}}

	tests := []struct {
		name           string
		layers         []EnvLayer
		env            map[string]string
		wantK          string
		wantAlreadySet int
	}{
		{
			name:   "a single layer is passed through",
			layers: []EnvLayer{store}, wantK: "from-store",
		},
		{
			name:   "the env file overlays the project store",
			layers: []EnvLayer{store, file}, wantK: "from-file",
		},
		{
			name:   "a value set for this run outranks every layer",
			layers: []EnvLayer{store, file},
			env:    map[string]string{"K": "from-shell"},
			wantK:  "from-shell", wantAlreadySet: 1,
		},
		{
			name:   "an empty value is leftover rather than a choice",
			layers: []EnvLayer{store, file},
			env:    map[string]string{"K": ""},
			wantK:  "from-file",
		},
		{
			name: "an empty value in a higher layer leaves a lower layer's value alone",
			layers: []EnvLayer{
				store,
				{Name: "file", Vars: map[string]string{"K": "", "ONLY_FILE": "f"}},
			},
			wantK: "from-store",
		},
		{
			name: "a key only ever supplied empty still defers to the environment",
			layers: []EnvLayer{
				{Name: "file", Vars: map[string]string{"K": ""}},
			},
			env:   map[string]string{"K": "from-shell"},
			wantK: "from-shell", wantAlreadySet: 1,
		},
		{
			name:   "the environment is consulted only for keys a layer supplies",
			layers: []EnvLayer{file},
			env:    map[string]string{"K": "from-shell", "UNRELATED": "noise"},
			wantK:  "from-shell", wantAlreadySet: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(k string) string { return tt.env[k] }

			merged, alreadySet := LayerEnv(lookup, tt.layers...)

			assert.Equal(t, tt.wantK, merged["K"], "the last layer to supply a key wins, unless the environment did")
			assert.Equal(t, tt.wantAlreadySet, alreadySet, "only keys deferred to the environment are counted")
			assert.NotContains(t, merged, "UNRELATED", "the whole environment must not leak into the result")
		})
	}
}

func TestLayerEnv_NoEnvLookupSkipsTheEnvironment(t *testing.T) {
	merged, alreadySet := LayerEnv(NoEnvLookup,
		EnvLayer{Name: "low", Vars: map[string]string{"K": "low"}},
		EnvLayer{Name: "high", Vars: map[string]string{"K": "high"}},
	)

	assert.Equal(t, "high", merged["K"], "layers still resolve without an environment")
	assert.Zero(t, alreadySet, "a caller with no environment defers to nothing")
}
