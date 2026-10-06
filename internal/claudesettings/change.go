package claudesettings

import (
	"fmt"
	"sort"
)

// Change records what Apply did to one settings file, so Undo can reverse it.
type Change struct {
	Path string             `json:"path"`
	Env  map[string]EnvEdit `json:"env"`
}

// EnvEdit is one env key Apply wrote. Previous is nil when the key was absent
// before the first Apply, so Undo removes it rather than restoring a value.
type EnvEdit struct {
	Wrote    string  `json:"wrote"`
	Previous *string `json:"previous"`
	// HeaderNames are the headers Apply added, for EnvCustomHeaders only. Undo
	// removes just these, so headers the user set themselves survive.
	HeaderNames []string `json:"header_names,omitempty"`
}

// Conflict is an env key that already holds a value Apply would overwrite and
// did not write itself.
type Conflict struct {
	Key      string
	Current  string
	Proposed string
}

// Conflicts lists the keys in want whose current value Apply would replace.
// Headers never conflict, because Apply merges them. A value the prior Change
// wrote is not a conflict, so re-running Apply is safe.
func Conflicts(f *File, want map[string]string, prior *Change) ([]Conflict, error) {
	var out []Conflict
	for _, key := range sortedKeys(want) {
		if key == EnvCustomHeaders {
			continue
		}
		current, present, err := f.Env(key)
		if err != nil {
			return nil, err
		}
		if !present || current == "" || current == want[key] || ownedBy(prior, key, current) {
			continue
		}
		out = append(out, Conflict{Key: key, Current: current, Proposed: want[key]})
	}
	return out, nil
}

// Apply writes want into the env block and returns the Change that undoes it.
// Passing the prior Change keeps the values that were there before the first
// Apply, so Undo after several runs still restores the original state.
func Apply(f *File, want map[string]string, prior *Change) (*Change, error) {
	change := &Change{Path: f.Path, Env: map[string]EnvEdit{}}
	for _, key := range sortedKeys(want) {
		current, present, err := f.Env(key)
		if err != nil {
			return nil, err
		}
		edit := EnvEdit{Previous: previousFor(prior, key, current, present)}
		if key == EnvCustomHeaders {
			merged := MergeHeaders(current, want[key])
			edit.Wrote = want[key]
			edit.HeaderNames = HeaderNames(want[key])
			if err := f.SetEnv(key, merged); err != nil {
				return nil, err
			}
		} else {
			edit.Wrote = want[key]
			if err := f.SetEnv(key, want[key]); err != nil {
				return nil, err
			}
		}
		change.Env[key] = edit
	}
	return change, nil
}

// Undo reverses c. A key whose value changed since Apply wrote it is left
// alone and returned in kept, because someone else now owns that value.
func Undo(f *File, c *Change) (kept []string, err error) {
	if c == nil {
		return nil, nil
	}
	for _, key := range sortedKeys(c.Env) {
		edit := c.Env[key]
		current, present, err := f.Env(key)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		if key == EnvCustomHeaders {
			rest := RemoveHeaders(current, edit.HeaderNames)
			if edit.Previous != nil {
				rest = MergeHeaders(rest, KeepHeaders(*edit.Previous, edit.HeaderNames))
			}
			if rest == "" && edit.Previous == nil {
				err = f.DeleteEnv(key)
			} else {
				err = f.SetEnv(key, rest)
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		if current != edit.Wrote {
			kept = append(kept, key)
			continue
		}
		if edit.Previous == nil {
			err = f.DeleteEnv(key)
		} else {
			err = f.SetEnv(key, *edit.Previous)
		}
		if err != nil {
			return nil, fmt.Errorf("restore %s: %w", key, err)
		}
	}
	return kept, nil
}

func ownedBy(prior *Change, key, current string) bool {
	if prior == nil {
		return false
	}
	edit, ok := prior.Env[key]
	return ok && edit.Wrote == current
}

func previousFor(prior *Change, key, current string, present bool) *string {
	if prior != nil {
		if edit, ok := prior.Env[key]; ok {
			return edit.Previous
		}
	}
	if !present {
		return nil
	}
	v := current
	return &v
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
