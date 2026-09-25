package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	composeBuilder "github.com/astropods/astro-cli/internal/compose"
	spec "github.com/astropods/astro-spec"
)

type validationError struct {
	message string
	line    int
}

// outf writes one line of command output. It owns the dropped write error, so
// the call sites below read as output rather than as error handling.
func outf(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, format, args...) //nolint:errcheck,gosec
}

// specSource is the spec file as read and parsed once, so a caller anchoring a
// finding to a line does not read and unmarshal it again.
type specSource struct {
	path  string
	lines []string
	root  *yaml.Node
}

func runValidate(w io.Writer, specPath string) error {
	outf(w, "\n")
	outf(w, "%s%sValidating %s...%s\n\n", colorBold, colorBlue, filepath.Base(specPath), colorReset)

	parsed, src, err := validateSpecFile(w, specPath)
	if err != nil {
		return err
	}

	if err := reportWatchDirs(w, parsed, src); err != nil {
		return err
	}

	outf(w, "%s✓%s %s is valid\n\n", colorGreen, colorReset, filepath.Base(specPath))
	return nil
}

// reportWatchDirs checks each dev.watch entry as a string and prints the ones
// that cannot name a directory, anchored to the entry's own line.
//
// It checks nothing on disk. Whether a directory exists is a property of the
// checkout, not of the spec, so it belongs to 'project start', which reports it
// and carries on. Validating it here would fail a spec that is correct, in a
// tree that merely has not been populated yet.
//
// A malformed entry is an error: it cannot work in any checkout. A repeated
// entry is a warning, because the builder mounts it once either way. A spec
// naming no directory is not checked, matching the builder's silence about the
// implicit agent/ default.
func reportWatchDirs(w io.Writer, parsed *spec.AstroSpec, src specSource) error {
	if parsed == nil || parsed.Dev == nil || len(parsed.Dev.Watch) == 0 {
		return nil
	}

	var errs []validationError
	seen := map[string]bool{}
	warnings := 0

	for i, entry := range parsed.Dev.Watch {
		line := findLineForDotPath(src.root, fmt.Sprintf("dev.watch[%d]", i))

		if reason := composeBuilder.InvalidWatchEntry(entry); reason != "" {
			errs = append(errs, validationError{
				message: composeBuilder.MsgWatchDirRejected(entry, reason),
				line:    line,
			})
			continue
		}
		// Only a well-formed entry has a cleaned form to compare, so the
		// duplicate check runs after the string check rather than beside it.
		dir := path.Clean(entry)
		if seen[dir] {
			outf(w, "  %s⚠%s %s\n", colorYellow, colorReset, composeBuilder.MsgWatchDirDuplicate(entry))
			warnings++
			continue
		}
		seen[dir] = true
	}

	if len(errs) == 0 {
		if warnings > 0 {
			outf(w, "\n")
		}
		return nil
	}
	for _, e := range errs {
		printValidationError(w, e, src.lines, filepath.Base(src.path))
	}
	outf(w, "%s%d error(s) found%s\n\n", colorRed, len(errs), colorReset)
	return fmt.Errorf("validation failed")
}

// validateSpecFile runs strict validation (YAML parse + JSON schema + semantic)
// on the spec at specPath. On failure, error details are printed to stdout and
// a non-nil error is returned. On success, returns the parsed *spec.AstroSpec
// and the source it was read from, so a caller can anchor further findings to a
// line without reading the file again.
func validateSpecFile(w io.Writer, specPath string) (*spec.AstroSpec, specSource, error) {
	data, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return nil, specSource{}, fmt.Errorf("cannot read %s: %w", filepath.Base(specPath), err)
	}

	lines := strings.Split(string(data), "\n")

	// YAML parse check
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		outf(w, "%s✗%s YAML syntax error: %v\n\n", colorRed, colorReset, err)
		return nil, specSource{}, fmt.Errorf("validation failed")
	}

	var rootNode yaml.Node
	_ = yaml.Unmarshal(data, &rootNode)
	src := specSource{path: specPath, lines: lines, root: &rootNode}

	var errs []validationError

	// JSON schema validation
	errs = append(errs, schemaValidationErrors(raw, &rootNode)...)

	// Semantic validation (required fields, mutual exclusions, etc.)
	parsed, semErr := spec.ParseSpec(specPath)
	if semErr != nil {
		msg := semErr.Error()
		line := 0
		if idx := strings.Index(msg, ": "); idx > 0 {
			line = findLineForDotPath(&rootNode, msg[:idx])
		}
		errs = append(errs, validationError{message: msg, line: line})
	}

	if len(errs) > 0 {
		for _, e := range errs {
			printValidationError(w, e, lines, filepath.Base(specPath))
		}
		outf(w, "%s%d error(s) found%s\n\n", colorRed, len(errs), colorReset)
		return nil, specSource{}, fmt.Errorf("validation failed")
	}

	// Non-fatal deprecation notices.
	if parsed != nil {
		for _, note := range spec.DeprecationWarnings(parsed) {
			outf(w, "  %s⚠%s %s\n", colorYellow, colorReset, note)
		}
	}

	return parsed, src, nil
}

func printValidationError(w io.Writer, e validationError, lines []string, filename string) {
	outf(w, "  %s✗%s %s\n", colorRed, colorReset, e.message)
	if e.line <= 0 || e.line > len(lines) {
		return
	}
	lineIdx := e.line - 1
	start := lineIdx - 2
	if start < 0 {
		start = 0
	}
	end := lineIdx + 3
	if end > len(lines) {
		end = len(lines)
	}
	outf(w, "\n    %s%s:%d%s\n", colorCyan, filename, e.line, colorReset)
	for i := start; i < end; i++ {
		if i == lineIdx {
			outf(w, "    %s%s> %4d │ %s%s\n", colorBold, colorRed, i+1, lines[i], colorReset)
		} else {
			outf(w, "    %s  %4d │ %s%s\n", colorDim, i+1, lines[i], colorReset)
		}
	}
	outf(w, "\n")
}

func schemaValidationErrors(raw interface{}, rootNode *yaml.Node) []validationError {
	// YAML → JSON to get standard JSON types expected by the validator
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return []validationError{{message: fmt.Sprintf("failed to convert spec to JSON: %v", err)}}
	}
	var jsonVal interface{}
	if err := json.Unmarshal(jsonBytes, &jsonVal); err != nil {
		return []validationError{{message: fmt.Sprintf("failed to re-parse spec as JSON: %v", err)}}
	}

	var schemaDoc interface{}
	if err := json.Unmarshal(spec.Schema(), &schemaDoc); err != nil {
		return []validationError{{message: fmt.Sprintf("failed to parse schema: %v", err)}}
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("astropods.schema.json", schemaDoc); err != nil {
		return []validationError{{message: fmt.Sprintf("failed to load schema: %v", err)}}
	}
	schema, err := c.Compile("astropods.schema.json")
	if err != nil {
		return []validationError{{message: fmt.Sprintf("failed to compile schema: %v", err)}}
	}

	valErr := schema.Validate(jsonVal)
	if valErr == nil {
		return nil
	}

	var ve *jsonschema.ValidationError
	if !errors.As(valErr, &ve) {
		return []validationError{{message: valErr.Error()}}
	}

	return collectSchemaErrors(ve, rootNode)
}

// collectSchemaErrors extracts individual error messages from the flattened basic output.
func collectSchemaErrors(ve *jsonschema.ValidationError, rootNode *yaml.Node) []validationError {
	basic := ve.BasicOutput()

	var results []validationError
	for _, unit := range basic.Errors {
		if unit.Error == nil {
			continue
		}
		msgBytes, err := json.Marshal(unit.Error)
		if err != nil {
			continue
		}
		msg := strings.Trim(string(msgBytes), `"`)
		if msg == "" {
			continue
		}
		loc := unit.InstanceLocation
		if loc == "" || loc == "/" {
			loc = "(root)"
		}
		line := findLineForJSONPointer(rootNode, unit.InstanceLocation)
		results = append(results, validationError{
			message: fmt.Sprintf("%s: %s", loc, msg),
			line:    line,
		})
	}

	// Fall back to the top-level error string if no leaf errors were extracted
	if len(results) == 0 {
		results = append(results, validationError{message: ve.Error()})
	}
	return results
}

// findLineForJSONPointer returns the YAML line for a JSON pointer path like "/agent/build/context".
func findLineForJSONPointer(root *yaml.Node, path string) int {
	if root == nil || root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return 0
	}
	if path == "" || path == "/" {
		return root.Content[0].Line
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	node := findNodeByPath(root.Content[0], parts)
	if node == nil {
		return 0
	}
	return node.Line
}

// findLineForDotPath returns the YAML line for a dot-notation path like "agent.inputs[0].name".
func findLineForDotPath(root *yaml.Node, dotPath string) int {
	if root == nil || root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return 0
	}
	node := findNodeByPath(root.Content[0], splitDotPath(dotPath))
	if node == nil {
		return 0
	}
	return node.Line
}

// splitDotPath converts "agent.inputs[0].name" to ["agent", "inputs", "0", "name"].
func splitDotPath(path string) []string {
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	return strings.Split(path, ".")
}

// findNodeByPath traverses a yaml.Node tree following the given path segments.
func findNodeByPath(node *yaml.Node, parts []string) *yaml.Node {
	if node == nil || len(parts) == 0 {
		return node
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	key := parts[0]
	rest := parts[1:]
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				return findNodeByPath(node.Content[i+1], rest)
			}
		}
	case yaml.SequenceNode:
		idx, err := strconv.Atoi(key)
		if err == nil && idx >= 0 && idx < len(node.Content) {
			return findNodeByPath(node.Content[idx], rest)
		}
	}
	return nil
}
