package evalpreset

import (
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
)

// RefDefaultSet is the evaluation set an agent resolves to when it has no
// published set of its own.
const RefDefaultSet = "preset/default-evaluation"

var sets = map[string][]string{
	RefDefaultSet: {
		RefExposedPII,
		RefLeakedCredentials,
		RefDisclosedSystemInstructions,
		RefUnnecessaryToolCall,
		RefClaimGrounding,
		RefUserSentiment,
	},
}

// SetRefs returns the ordered evaluator references a preset set contains.
func SetRefs(ref string) ([]string, error) {
	set, ok := sets[ref]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownRef, ref)
	}
	return append([]string(nil), set...), nil
}

// ResolveSet returns the evaluators a preset set contains, in definition order.
func ResolveSet(ref string) ([]evaluator.Evaluator, error) {
	refs, err := SetRefs(ref)
	if err != nil {
		return nil, err
	}
	out := make([]evaluator.Evaluator, 0, len(refs))
	for _, evaluatorRef := range refs {
		preset, err := Lookup(evaluatorRef)
		if err != nil {
			return nil, fmt.Errorf("resolve set %s: %w", ref, err)
		}
		out = append(out, preset)
	}
	return out, nil
}

// IsSetRef reports whether the reference names a preset evaluation set.
func IsSetRef(ref string) bool {
	_, ok := sets[ref]
	return ok
}
