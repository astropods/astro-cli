package claudesettings

import (
	"fmt"
	"sort"
)

type Change struct {
	Path string             `json:"path"`
	Env  map[string]EnvEdit `json:"env"`
}

// Previous is nil when the key was absent before the first Apply.
type EnvEdit struct {
	Wrote    string  `json:"wrote"`
	Previous *string `json:"previous"`
	// HeaderNames is set for EnvCustomHeaders only.
	HeaderNames []string `json:"header_names,omitempty"`
}

type Conflict struct {
	Key      string
	Current  string
	Proposed string
}

// Headers never conflict, and neither does a value the prior Change wrote.
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

// Passing the prior Change keeps the values from before the first Apply.
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

// Undo leaves a key whose value changed since Apply and returns it in kept.
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
