package evaldocument

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
)

const (
	schemaV1 = "evaluation/v1"

	minEvaluators = 1
	maxEvaluators = 10

	maxTotalBytes = 128 * 1024
)

var ErrInvalidDocument = errors.New("invalid evaluation document")

type Entry struct {
	Ref         string            `json:"ref,omitempty"`
	Key         string            `json:"key,omitempty"`
	Label       string            `json:"label,omitempty"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	Config      *evaluator.Config `json:"config,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Output      *evaluator.Output `json:"output,omitempty"`
}

type Document struct {
	Schema     string  `json:"schema"`
	Evaluators []Entry `json:"evaluators"`
}

type Result struct {
	EvaluationRef string
	Document      Document
	Evaluators    []evaluator.Evaluator
}

type rawDocument struct {
	Schema     string     `yaml:"schema"`
	Evaluators []rawEntry `yaml:"evaluators"`
}

type rawEntry struct {
	Ref         string     `yaml:"ref"`
	Key         string     `yaml:"key"`
	Label       string     `yaml:"label"`
	Description string     `yaml:"description"`
	Type        string     `yaml:"type"`
	Config      *rawConfig `yaml:"config"`
	Prompt      string     `yaml:"prompt"`
	PromptFile  string     `yaml:"prompt_file"`
	Output      *rawOutput `yaml:"output"`
}

type rawConfig struct {
	Context *rawContext `yaml:"context"`
}

type rawContext struct {
	PreviousTurns   *bool    `yaml:"previous_turns"`
	NextUserMessage *bool    `yaml:"next_user_message"`
	UserFeedback    *bool    `yaml:"user_feedback"`
	Steps           *bool    `yaml:"steps"`
	StepTypes       []string `yaml:"step_types"`
}

type rawOutput struct {
	Type      string   `yaml:"type"`
	Options   []string `yaml:"options"`
	Minimum   *float64 `yaml:"minimum"`
	Maximum   *float64 `yaml:"maximum"`
	MaxLength *int     `yaml:"max_length"`
}

func Parse(yamlText string, promptFiles map[string]string) (Result, error) {
	if size := totalBytes(yamlText, promptFiles); size > maxTotalBytes {
		return Result{}, invalidDocument("document and prompt files total %d bytes, exceeding %d", size, maxTotalBytes)
	}

	raw, err := decodeStrict(yamlText)
	if err != nil {
		return Result{}, err
	}

	if raw.Schema != schemaV1 {
		return Result{}, invalidDocument("schema must be %q, got %q", schemaV1, raw.Schema)
	}
	if len(raw.Evaluators) < minEvaluators || len(raw.Evaluators) > maxEvaluators {
		return Result{}, invalidDocument("document must contain %d to %d evaluators, got %d",
			minEvaluators, maxEvaluators, len(raw.Evaluators))
	}

	usedPromptFiles := make(map[string]bool, len(promptFiles))
	seenKeys := make(map[string]bool, len(raw.Evaluators))
	entries := make([]Entry, 0, len(raw.Evaluators))
	resolved := make([]evaluator.Evaluator, 0, len(raw.Evaluators))

	for i, r := range raw.Evaluators {
		entry, def, err := normalizeEntry(r, promptFiles, usedPromptFiles)
		if err != nil {
			return Result{}, invalidDocument("evaluator %d: %v", i, err)
		}
		if seenKeys[def.Key] {
			return Result{}, invalidDocument("duplicate evaluator key %q", def.Key)
		}
		seenKeys[def.Key] = true
		entries = append(entries, entry)
		resolved = append(resolved, def)
	}

	for filePath := range promptFiles {
		if !usedPromptFiles[filePath] {
			return Result{}, invalidDocument("prompt file %q is not referenced by any evaluator", filePath)
		}
	}

	doc := Document{Schema: raw.Schema, Evaluators: entries}
	ref, err := evaluationRef(doc)
	if err != nil {
		return Result{}, fmt.Errorf("evaldocument compute ref: %w", err)
	}

	return Result{EvaluationRef: ref, Document: doc, Evaluators: resolved}, nil
}

func decodeStrict(yamlText string) (rawDocument, error) {
	var raw rawDocument
	dec := yaml.NewDecoder(strings.NewReader(yamlText))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return rawDocument{}, invalidDocument("%v", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return rawDocument{}, invalidDocument("document must contain exactly one YAML document")
	}
	return raw, nil
}

func normalizeEntry(
	r rawEntry,
	promptFiles map[string]string,
	usedPromptFiles map[string]bool,
) (Entry, evaluator.Evaluator, error) {
	if strings.TrimSpace(r.Ref) != "" {
		return normalizeRefEntry(r)
	}
	return normalizeCustomEntry(r, promptFiles, usedPromptFiles)
}

func normalizeRefEntry(r rawEntry) (Entry, evaluator.Evaluator, error) {
	if r.Key != "" || r.Label != "" || r.Description != "" || r.Type != "" || r.Config != nil || r.Prompt != "" || r.PromptFile != "" || r.Output != nil {
		return Entry{}, evaluator.Evaluator{}, fmt.Errorf("a preset reference accepts no other fields")
	}
	if !evalpreset.IsEvaluatorRef(r.Ref) {
		return Entry{}, evaluator.Evaluator{}, fmt.Errorf("%w: %s", evalpreset.ErrUnknownRef, r.Ref)
	}
	def, err := evalpreset.Lookup(r.Ref)
	if err != nil {
		return Entry{}, evaluator.Evaluator{}, err
	}
	return Entry{Ref: r.Ref}, def, nil
}

func normalizeCustomEntry(
	r rawEntry,
	promptFiles map[string]string,
	usedPromptFiles map[string]bool,
) (Entry, evaluator.Evaluator, error) {
	if r.Key == "" && r.Label == "" && r.Type == "" && r.Prompt == "" && r.PromptFile == "" && r.Output == nil {
		return Entry{}, evaluator.Evaluator{}, fmt.Errorf("must be a preset reference or a complete custom definition")
	}

	prompt, err := resolvePrompt(r, promptFiles, usedPromptFiles)
	if err != nil {
		return Entry{}, evaluator.Evaluator{}, err
	}

	config := resolveConfig(r.Config)
	output := resolveOutput(r.Output)

	def := evaluator.Evaluator{
		Key:         r.Key,
		Label:       r.Label,
		Description: r.Description,
		Type:        evaluator.Type(r.Type),
		Config:      config,
		Prompt:      prompt,
		Output:      output,
	}
	if err := evaluator.Validate(def); err != nil {
		return Entry{}, evaluator.Evaluator{}, err
	}

	entry := Entry{
		Key:         def.Key,
		Label:       def.Label,
		Description: def.Description,
		Type:        string(def.Type),
		Config:      &config,
		Prompt:      def.Prompt,
		Output:      &output,
	}
	return entry, def, nil
}

func resolvePrompt(r rawEntry, promptFiles map[string]string, usedPromptFiles map[string]bool) (string, error) {
	hasPrompt := strings.TrimSpace(r.Prompt) != ""
	hasPromptFile := strings.TrimSpace(r.PromptFile) != ""
	if hasPrompt && hasPromptFile {
		return "", fmt.Errorf("must set exactly one of prompt or prompt_file")
	}
	if hasPrompt {
		return normalizeLineEndings(r.Prompt), nil
	}
	if !hasPromptFile {
		return "", fmt.Errorf("must set exactly one of prompt or prompt_file")
	}
	if !validPromptFilePath(r.PromptFile) {
		return "", fmt.Errorf("prompt_file %q must be a relative .md path within the project", r.PromptFile)
	}
	contents, ok := promptFiles[r.PromptFile]
	if !ok {
		return "", fmt.Errorf("prompt file %q was not provided", r.PromptFile)
	}
	if !utf8.ValidString(contents) {
		return "", fmt.Errorf("prompt file %q is not valid UTF-8", r.PromptFile)
	}
	usedPromptFiles[r.PromptFile] = true
	return normalizeLineEndings(contents), nil
}

func validPromptFilePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || !strings.HasSuffix(p, ".md") {
		return false
	}
	cleaned := path.Clean(p)
	return cleaned == p && cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func resolveConfig(c *rawConfig) evaluator.Config {
	if c == nil || c.Context == nil {
		return evaluator.Config{}
	}
	ctx := c.Context
	config := evaluator.Config{}
	if ctx.PreviousTurns != nil {
		config.Context.PreviousTurns = *ctx.PreviousTurns
	}
	if ctx.NextUserMessage != nil {
		config.Context.NextUserMessage = *ctx.NextUserMessage
	}
	if ctx.UserFeedback != nil {
		config.Context.UserFeedback = *ctx.UserFeedback
	}
	if ctx.Steps != nil {
		config.Context.Steps = *ctx.Steps
	}
	config.Context.StepTypes = ctx.StepTypes
	return config
}

func resolveOutput(o *rawOutput) evaluator.Output {
	if o == nil {
		return evaluator.Output{}
	}
	return evaluator.Output{
		Type:      evaluator.OutputType(o.Type),
		Options:   o.Options,
		Minimum:   o.Minimum,
		Maximum:   o.Maximum,
		MaxLength: o.MaxLength,
	}
}

// ResolveDocument expands a normalized Document (as stored in eval_definitions)
// into its executable evaluator set, resolving any embedded preset references
// against the current registry.
func ResolveDocument(doc Document) ([]evaluator.Evaluator, error) {
	out := make([]evaluator.Evaluator, 0, len(doc.Evaluators))
	for _, entry := range doc.Evaluators {
		if entry.Ref != "" {
			def, err := evalpreset.Lookup(entry.Ref)
			if err != nil {
				return nil, err
			}
			out = append(out, def)
			continue
		}
		config := evaluator.Config{}
		if entry.Config != nil {
			config = *entry.Config
		}
		output := evaluator.Output{}
		if entry.Output != nil {
			output = *entry.Output
		}
		out = append(out, evaluator.Evaluator{
			Key:    entry.Key,
			Label:  entry.Label,
			Type:   evaluator.Type(entry.Type),
			Config: config,
			Prompt: entry.Prompt,
			Output: output,
		})
	}
	return out, nil
}

func totalBytes(yamlText string, promptFiles map[string]string) int {
	total := len(yamlText)
	for _, contents := range promptFiles {
		total += len(contents)
	}
	return total
}

func invalidDocument(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDocument, fmt.Sprintf(format, args...))
}
