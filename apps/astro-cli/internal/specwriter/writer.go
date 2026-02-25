package specwriter

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SectionNames returns the set of existing entry names under section in the spec at path.
// Returns an empty map if the file or section does not exist.
func SectionNames(path, section string) map[string]bool {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return map[string]bool{}
	}
	var spec map[string]any
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return map[string]bool{}
	}
	names := map[string]bool{}
	if existing, ok := spec[section]; ok {
		if m, ok := existing.(map[string]any); ok {
			for k := range m {
				names[k] = true
			}
		}
	}
	return names
}

// AddEntry inserts entry under section[name] in the YAML spec at path.
//
// It reads the existing file with yaml.Node to find the correct insertion
// point by line number, then splices the new text in — the rest of the file
// is written back byte-for-byte so order, blank lines, and comments are
// preserved.
func AddEntry(path, section, name string, entry map[string]any) error {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Parse just enough to (a) check for duplicates and (b) find line numbers.
	var doc yaml.Node
	_ = yaml.Unmarshal(data, &doc)

	sectionExists := false
	// nextSectionLine is the 1-indexed line of the key that follows the target
	// section; -1 means the section is last (or the file is empty).
	nextSectionLine := -1

	if doc.Kind != 0 && len(doc.Content) > 0 {
		root := doc.Content[0]
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value == section {
				sectionExists = true
				secVal := root.Content[i+1]
				for j := 0; j+1 < len(secVal.Content); j += 2 {
					if secVal.Content[j].Value == name {
						return fmt.Errorf("%s %q already exists in %s", section, name, path)
					}
				}
				if i+2 < len(root.Content) {
					nextSectionLine = root.Content[i+2].Line
				}
				break
			}
		}
	}

	snippet, err := buildSnippet(name, entry)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	var result string
	switch {
	case !sectionExists:
		// Append a new section at the end of the file, separated by a blank line.
		result = strings.TrimRight(string(data), "\n") + "\n\n" + section + ":\n" + snippet

	case nextSectionLine < 0:
		// Target section is the last one — append to the end.
		result = strings.TrimRight(string(data), "\n") + "\n" + snippet

	default:
		// Find the insertion point: just before any trailing blank lines that
		// separate the target section from the next one.
		insertBefore := nextSectionLine - 1 // convert to 0-indexed
		for insertBefore > 0 && strings.TrimSpace(lines[insertBefore-1]) == "" {
			insertBefore--
		}
		before := strings.Join(lines[:insertBefore], "\n")
		after := strings.Join(lines[insertBefore:], "\n")
		result = before + "\n" + snippet + after
	}

	return os.WriteFile(path, []byte(result), 0600) //nolint:gosec
}

// buildSnippet marshals {name: entry} with 2-space indent and then shifts the
// entire block right by 2 spaces so it sits correctly inside a top-level section.
func buildSnippet(name string, entry map[string]any) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{name: entry}); err != nil {
		return "", fmt.Errorf("failed to marshal entry: %w", err)
	}
	_ = enc.Close()

	raw := strings.TrimRight(buf.String(), "\n")
	var sb strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			sb.WriteString("\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	return sb.String(), nil
}
