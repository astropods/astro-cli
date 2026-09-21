package utils

// EnvLayer is one source of environment values, named so a caller can narrate
// what each source contributed.
type EnvLayer struct {
	Name string
	Vars map[string]string
}

// NoEnvLookup is the lookup to pass when the process environment must not
// affect the result. LayerEnv then returns the layers alone.
var NoEnvLookup func(string) string

// LayerEnv merges layers in order, each overlaying the one before, so the last
// layer listed wins a conflict. It then lets the caller's environment win over
// every layer for the keys the layers supply, and reports how many keys it
// deferred to in that way.
//
// Only keys some layer supplies are consulted, so a caller gets the values its
// sources named rather than the whole environment.
//
// A value that is present but empty counts as unset, in a layer and in the
// environment alike: an empty higher layer leaves a lower layer's real value
// alone. The key still counts as supplied, so the environment is consulted for
// it.
//
// Pass NoEnvLookup to skip the environment entirely, for a caller layering
// values that have nothing to do with this process.
func LayerEnv(lookup func(string) string, layers ...EnvLayer) (map[string]string, int) {
	merged := make(map[string]string)
	for _, layer := range layers {
		for k, v := range layer.Vars {
			if v == "" && merged[k] != "" {
				continue
			}
			merged[k] = v
		}
	}
	if lookup == nil {
		return merged, 0
	}

	alreadySet := 0
	for k := range merged {
		if v := lookup(k); v != "" {
			merged[k] = v
			alreadySet++
		}
	}
	return merged, alreadySet
}
