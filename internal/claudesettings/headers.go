package claudesettings

import "strings"

// ANTHROPIC_CUSTOM_HEADERS holds one "Name: value" header per line.

// HeaderNames returns the names in a header block, lowercased, in order.
func HeaderNames(block string) []string {
	var names []string
	for _, line := range splitLines(block) {
		if name, _, ok := splitHeader(line); ok {
			names = append(names, strings.ToLower(name))
		}
	}
	return names
}

// MergeHeaders replaces every header in existing that ours also sets, keeps the
// rest in their original order, and appends ours in their own order.
func MergeHeaders(existing, ours string) string {
	replace := map[string]bool{}
	for _, n := range HeaderNames(ours) {
		replace[n] = true
	}
	kept := make([]string, 0, 4)
	for _, line := range splitLines(existing) {
		if name, _, ok := splitHeader(line); ok && replace[strings.ToLower(name)] {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(append(kept, splitLines(ours)...), "\n")
}

// RemoveHeaders drops every header in block whose name is in names.
func RemoveHeaders(block string, names []string) string {
	return filterHeaders(block, names, false)
}

// KeepHeaders returns only the headers in block whose name is in names.
func KeepHeaders(block string, names []string) string {
	return filterHeaders(block, names, true)
}

func filterHeaders(block string, names []string, keepNamed bool) string {
	named := map[string]bool{}
	for _, n := range names {
		named[strings.ToLower(n)] = true
	}
	kept := make([]string, 0, 4)
	for _, line := range splitLines(block) {
		name, _, ok := splitHeader(line)
		if (ok && named[strings.ToLower(name)]) == keepNamed {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// HeaderValue returns the value of the first header named name.
func HeaderValue(block, name string) (string, bool) {
	for _, line := range splitLines(block) {
		if n, v, ok := splitHeader(line); ok && strings.EqualFold(n, name) {
			return v, true
		}
	}
	return "", false
}

func splitLines(block string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(block, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func splitHeader(line string) (name, value string, ok bool) {
	name, value, ok = strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(name), strings.TrimSpace(value), strings.TrimSpace(name) != ""
}
