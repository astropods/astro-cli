package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/aymanbagabas/go-udiff"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
	repairui "github.com/astropods/astro/apps/astro-cli/internal/tui/repair"
	spec "github.com/astropods/astro/packages/astro-spec"
)

type repairFileCheck struct {
	path         string
	templatePath string
	static       bool // copy verbatim, skip template rendering
}

// runRepair expects an already-resolved spec path and working directory; the caller is responsible for resolution.
func runRepair(specPath, workingDir string) error {
	var astroSpec *spec.AstroSpec
	var specErr error
	if specPath != "" {
		astroSpec, specErr = spec.ParseSpec(specPath)
	} else {
		specErr = errNoSpecFile
	}

	var config scaffold.ScaffoldConfig
	var specMissing bool

	if specErr != nil {
		// Check if the old astro.yml filename is present and offer to rename it.
		oldSpecPath := filepath.Join(workingDir, "astro.yml")
		if oldSpec, oldErr := spec.ParseSpec(oldSpecPath); oldErr == nil {
			targetName := SpecFileAliases[0]
			targetPath := filepath.Join(workingDir, targetName)
			fmt.Println()
			fmt.Printf("%s!%s Found %sastro.yml%s — this file has been renamed to %s%s%s\n",
				colorYellow, colorReset, colorBold, colorReset, colorBold, targetName, colorReset)

			rename := true
			if !yesFlag {
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("  Rename astro.yml → %s? [Y/n] ", targetName)
				line, _ := reader.ReadString('\n')
				trimmed := strings.TrimSpace(strings.ToLower(line))
				if trimmed != "" && trimmed != "y" {
					rename = false
					fmt.Printf("  %sskipped%s\n", colorDim, colorReset)
				}
			}

			if rename {
				if renameErr := os.Rename(oldSpecPath, targetPath); renameErr != nil {
					fmt.Printf("  %s✗%s failed to rename: %v\n", colorRed, colorReset, renameErr)
				} else {
					fmt.Printf("  %s✓%s Renamed astro.yml → %s\n", colorGreen, colorReset, targetName)
					astroSpec = oldSpec
					specPath = targetPath
					specErr = nil
				}
			} else {
				// Use the old spec for config even without renaming
				astroSpec = oldSpec
				specPath = oldSpecPath
				specErr = nil
			}
		}

		if specErr != nil {
			specMissing = true
			config = inferConfigFallback(workingDir)
			fmt.Println()
			fmt.Printf("%s!%s spec file is missing or invalid — using inferred config (name: %s%s%s)\n",
				colorYellow, colorReset, colorBold, config.Name, colorReset)
		}
	}

	if specErr == nil {
		config = configFromSpec(astroSpec)
	}

	paths, err := scaffold.GetTemplatePaths("mastra")
	if err != nil {
		return err
	}

	type fileEntry struct {
		check           repairFileCheck
		label           string
		defaultSelected bool
	}

	entries := []fileEntry{
		{repairFileCheck{filepath.Join(workingDir, "Dockerfile"), paths.Dockerfile, false}, "Dockerfile", true},
		{repairFileCheck{filepath.Join(workingDir, "tsconfig.json"), paths.Tsconfig, false}, "tsconfig.json", true},
		{repairFileCheck{filepath.Join(workingDir, ".gitignore"), paths.Gitignore, false}, ".gitignore", true},
		{repairFileCheck{filepath.Join(workingDir, ".dockerignore"), paths.Dockerignore, false}, ".dockerignore", true},
		{repairFileCheck{filepath.Join(workingDir, "CLAUDE.md"), paths.LlmMd, false}, "CLAUDE.md", true},
		{repairFileCheck{filepath.Join(workingDir, "AGENTS.md"), paths.LlmMd, false}, "AGENTS.md", true},
	}

	for _, ing := range config.Ingestions {
		ingestionTemplate := paths.IngestionIndex
		if ing == "webhook" {
			ingestionTemplate = paths.IngestionWebhookIndex
		}
		entries = append(entries,
			fileEntry{repairFileCheck{filepath.Join(workingDir, "ingestion", ing, "Dockerfile"), paths.DockerfileIngestion, false}, filepath.Join("ingestion", ing, "Dockerfile"), true},
			fileEntry{repairFileCheck{filepath.Join(workingDir, "ingestion", ing, "index.ts"), ingestionTemplate, false}, filepath.Join("ingestion", ing, "index.ts"), true},
		)
	}

	entries = append(entries,
		fileEntry{repairFileCheck{filepath.Join(workingDir, "postman", "collections", "messaging.postman_collection.json"), paths.PostmanCollection, true}, filepath.Join("postman", "collections", "messaging.postman_collection.json"), true},
	)
	if config.HasIngestion("webhook") {
		entries = append(entries,
			fileEntry{repairFileCheck{filepath.Join(workingDir, "postman", "collections", "webhook.postman_collection.json"), paths.PostmanWebhookCollection, true}, filepath.Join("postman", "collections", "webhook.postman_collection.json"), true},
		)
	}

	// Ask the user which files to repair (skipped when --yes)
	if !yesFlag {
		items := make([]repairui.Item, len(entries))
		for i, e := range entries {
			items[i] = repairui.Item{Label: e.label, Selected: e.defaultSelected}
		}
		selected, err := repairui.Run(items)
		if err != nil {
			return fmt.Errorf("repair cancelled")
		}
		// Rebuild entries keeping only selected files
		var filtered []fileEntry
		for i, e := range entries {
			if selected[i].Selected {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Build the final checks slice from entries
	checks := make([]repairFileCheck, len(entries))
	for i, e := range entries {
		checks[i] = e.check
	}

	fmt.Println()
	fmt.Printf("%s%sScanning project files...%s\n\n", colorBold, colorBlue, colorReset)

	reader := bufio.NewReader(os.Stdin)
	fixed, skipped := 0, 0

	// processIssue shows the diff (if any) and prompts to act, returning true if the file was written.
	processIssue := func(check repairFileCheck, relPath, action string) {
		if !yesFlag {
			fmt.Printf("  %s%s%s? [Y/n] ", colorBold, relPath, colorReset)
			line, _ := reader.ReadString('\n')
			trimmed := strings.TrimSpace(strings.ToLower(line))
			if trimmed != "" && trimmed != "y" {
				fmt.Printf("  %sskipped%s\n\n", colorDim, colorReset)
				skipped++
				return
			}
		}
		var writeErr error
		if check.static {
			writeErr = writeStaticRepairFile(check.path, check.templatePath)
		} else {
			writeErr = writeRepairFile(check.path, check.templatePath, config)
		}
		if writeErr != nil {
			fmt.Printf("  %s✗%s failed to %s: %v\n\n", colorRed, colorReset, strings.ToLower(action), writeErr)
		} else {
			fmt.Printf("  %s✓%s %sd\n\n", colorGreen, colorReset, action)
			fixed++
		}
	}

	// astroai.yml: only surfaced when missing, never compared to template
	if specMissing {
		relPath := filepath.Base(specPath)
		fmt.Printf("  %s✗%s %s %s(missing)%s\n", colorRed, colorReset, relPath, colorDim, colorReset)
		processIssue(repairFileCheck{specPath, paths.AstroYml, false}, relPath, "Create")
	}

	for _, check := range checks {
		var rendered string
		if check.static {
			data, err := scaffold.GetTemplate(check.templatePath)
			if err != nil {
				continue
			}
			rendered = data
		} else {
			var err error
			rendered, err = renderTemplateString(check.templatePath, config)
			if err != nil {
				continue
			}
		}

		relPath, _ := filepath.Rel(workingDir, check.path)
		existing, readErr := os.ReadFile(check.path) //nolint:gosec

		switch {
		case os.IsNotExist(readErr):
			fmt.Printf("  %s✗%s %s %s(missing)%s\n", colorRed, colorReset, relPath, colorDim, colorReset)
			processIssue(check, relPath, "Create")
		case readErr != nil:
			fmt.Printf("  %s!%s %s %s(unreadable)%s\n\n", colorYellow, colorReset, relPath, colorDim, colorReset)
		case string(existing) != rendered:
			fmt.Printf("  %s~%s %s %s(differs from template)%s\n", colorYellow, colorReset, relPath, colorDim, colorReset)
			printDiff(relPath, string(existing), rendered)
			fmt.Println()
			processIssue(check, relPath, "Update")
		default:
			fmt.Printf("  %s✓%s %s\n", colorGreen, colorReset, relPath)
		}
	}

	fmt.Println()
	if fixed > 0 {
		fmt.Printf("%s✓%s %d file(s) repaired\n\n", colorGreen, colorReset, fixed)
	} else if skipped == 0 {
		fmt.Printf("%s✓%s All files are up to date\n\n", colorGreen, colorReset)
	}

	// Scan for deprecated references
	checkDeprecatedPackages(workingDir, reader, yesFlag)

	// Remove deprecated build args/secrets from astroai.yml
	if !specMissing {
		checkBuildArgsAndSecrets(specPath, reader, yesFlag)
	}

	// Delete the old .astro directory if present (replaced by .ast).
	astroDir := filepath.Join(workingDir, ".astro")
	if _, err := os.Stat(astroDir); err == nil {
		if removeErr := os.RemoveAll(astroDir); removeErr != nil {
			fmt.Printf("%s!%s Failed to remove .astro: %v\n", colorYellow, colorReset, removeErr)
		} else {
			fmt.Printf("%s✓%s Removed .astro\n", colorGreen, colorReset)
		}
	}

	return nil
}

func renderTemplateString(templatePath string, config scaffold.ScaffoldConfig) (string, error) {
	tmplStr, err := scaffold.GetTemplate(templatePath)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeStaticRepairFile(outputPath, embedPath string) error {
	data, err := scaffold.GetTemplate(embedPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil { //nolint:gosec
		return err
	}
	return os.WriteFile(outputPath, []byte(data), 0644) //nolint:gosec
}

func writeRepairFile(outputPath, templatePath string, config scaffold.ScaffoldConfig) error {
	rendered, err := renderTemplateString(templatePath, config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil { //nolint:gosec
		return err
	}
	return os.WriteFile(outputPath, []byte(rendered), 0644) //nolint:gosec
}

// inferConfigFallback builds a minimal ScaffoldConfig when astroai.yml is absent or unparseable.
// It tries to read the project name and description from package.json, falling back to the directory name.
func inferConfigFallback(workingDir string) scaffold.ScaffoldConfig {
	config := scaffold.DefaultConfig(filepath.Base(workingDir))

	pkgPath := filepath.Join(workingDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil { //nolint:gosec
		var pkg struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			if pkg.Name != "" {
				config.Name = pkg.Name
			}
			if pkg.Description != "" {
				config.Description = pkg.Description
			}
		}
	}

	return config
}

// printDiff renders a colored unified diff between current and template content.
func printDiff(name, current, template string) {
	diff := udiff.Unified("current/"+name, "template/"+name, current, template)
	if diff == "" {
		return
	}
	for _, line := range strings.SplitAfter(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			fmt.Printf("    %s%s%s", colorBold, line, colorReset)
		case strings.HasPrefix(line, "@@"):
			fmt.Printf("    %s%s%s", colorCyan, line, colorReset)
		case strings.HasPrefix(line, "+"):
			fmt.Printf("    %s%s%s", colorGreen, line, colorReset)
		case strings.HasPrefix(line, "-"):
			fmt.Printf("    %s%s%s", colorRed, line, colorReset)
		default:
			fmt.Printf("    %s", line)
		}
	}
}

func configFromSpec(s *spec.AstroSpec) scaffold.ScaffoldConfig {
	config := scaffold.ScaffoldConfig{
		IntegrationKeys: map[string]string{},
		Name:            s.Name,
		Description:     "",
	}

	// Interfaces from dev section
	config.Interfaces = s.Dev.MessagingAdapters()
	if len(config.Interfaces) == 0 {
		config.Interfaces = []string{"web"}
	}

	// Self-hosted model provider and model name
	for _, model := range s.Models {
		p := strings.ToLower(model.Provider)
		if p == "ollama" || p == "huggingface" {
			config.ModelProvider = model.Provider
			if models := model.ResolvedModels(); len(models) > 0 {
				config.Model = models[0]
			}
			break
		}
	}

	// Integrations: cloud model providers + custom providers
	integrationSet := map[string]bool{}
	for _, model := range s.Models {
		p := strings.ToLower(model.Provider)
		if p == "anthropic" || p == "openai" {
			integrationSet[p] = true
		}
	}
	for name := range s.Providers {
		integrationSet[strings.ToLower(name)] = true
	}
	for k := range integrationSet {
		config.Integrations = append(config.Integrations, k)
	}

	// Knowledge store providers
	for _, k := range s.Knowledge {
		if k.Provider != "" {
			config.Knowledge = append(config.Knowledge, strings.ToLower(k.Provider))
		}
	}

	// Ingestion trigger types
	for _, ing := range s.Ingestion {
		config.Ingestions = append(config.Ingestions, ing.Trigger.Type)
	}

	return config
}

// checkDeprecatedPackages scans project files for astro-messaging and prompts to update.
func checkDeprecatedPackages(workingDir string, reader *bufio.Reader, yes bool) {
	const oldPkg = "@astromode-ai/astro-messaging"
	const newPkg = "@astropods/messaging"

	pattern := regexp.MustCompile(regexp.QuoteMeta(oldPkg))

	var matches []struct {
		file string
		line int
		text string
	}

	_ = filepath.WalkDir(workingDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".ast" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ts" && ext != ".js" && ext != ".json" {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return nil
		}
		for i, l := range strings.Split(string(data), "\n") {
			if pattern.MatchString(l) {
				rel, _ := filepath.Rel(workingDir, path)
				matches = append(matches, struct {
					file string
					line int
					text string
				}{rel, i + 1, strings.TrimSpace(l)})
			}
		}
		return nil
	})

	if len(matches) == 0 {
		return
	}

	fmt.Println()
	fmt.Printf("%s!%s Found deprecated package %s%s%s (renamed to %s%s%s):\n",
		colorYellow, colorReset, colorBold, oldPkg, colorReset, colorBold, newPkg, colorReset)
	fmt.Println()
	for _, m := range matches {
		fmt.Printf("  %s%s:%d%s  %s%s%s\n", colorDim, m.file, m.line, colorReset, colorDim, m.text, colorReset)
	}
	fmt.Println()

	if !yes {
		fmt.Printf("  Replace all occurrences with %s? [Y/n] ", newPkg)
		line, _ := reader.ReadString('\n')
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if trimmed != "" && trimmed != "y" {
			fmt.Printf("  %sskipped%s\n\n", colorDim, colorReset)
			return
		}
	}

	seen := map[string]bool{}
	updated := 0
	for _, m := range matches {
		if seen[m.file] {
			continue
		}
		seen[m.file] = true
		fullPath := filepath.Join(workingDir, m.file)
		data, err := os.ReadFile(fullPath) //nolint:gosec
		if err != nil {
			continue
		}
		newData := pattern.ReplaceAll(data, []byte(newPkg))
		if err := os.WriteFile(fullPath, newData, 0644); err != nil { //nolint:gosec
			fmt.Printf("  %s✗%s failed to update %s: %v\n", colorRed, colorReset, m.file, err)
		} else {
			fmt.Printf("  %s✓%s updated %s\n", colorGreen, colorReset, m.file)
			updated++
		}
	}
	if updated > 0 {
		fmt.Println()
	}
}

// checkBuildArgsAndSecrets scans astroai.yml for deprecated build args/secrets under
// agent.build and ingestion[*].container.build, and offers to remove them.
func checkBuildArgsAndSecrets(specPath string, reader *bufio.Reader, yes bool) {
	data, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 {
		return
	}
	doc := root.Content[0]

	removed := removeBuildArgsSecrets(doc)
	if removed == 0 {
		return
	}

	fmt.Println()
	fmt.Printf("%s!%s Found deprecated %sargs%s/%ssecrets%s in build config in %s (no longer needed):\n",
		colorYellow, colorReset, colorBold, colorReset, colorBold, colorReset, filepath.Base(specPath))
	fmt.Println()

	if !yes {
		fmt.Printf("  Remove build args and secrets from %s? [Y/n] ", filepath.Base(specPath))
		line, _ := reader.ReadString('\n')
		trimmed := strings.TrimSpace(strings.ToLower(line))
		if trimmed != "" && trimmed != "y" {
			fmt.Printf("  %sskipped%s\n\n", colorDim, colorReset)
			return
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		fmt.Printf("  %s✗%s failed to encode yaml: %v\n", colorRed, colorReset, err)
		return
	}
	if err := os.WriteFile(specPath, buf.Bytes(), 0600); err != nil { //nolint:gosec
		fmt.Printf("  %s✗%s failed to write %s: %v\n", colorRed, colorReset, filepath.Base(specPath), err)
		return
	}
	fmt.Printf("  %s✓%s Removed build args and secrets from %s\n\n", colorGreen, colorReset, filepath.Base(specPath))
}

// removeBuildArgsSecrets walks a yaml.Node tree and removes "args" and "secrets" keys
// from any "build" mapping node. Returns the number of keys removed.
func removeBuildArgsSecrets(node *yaml.Node) int {
	if node == nil {
		return 0
	}
	removed := 0
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]
			if key.Value == "build" && val.Kind == yaml.MappingNode {
				// Remove args and secrets keys from this build node
				var kept []*yaml.Node
				for j := 0; j < len(val.Content)-1; j += 2 {
					if val.Content[j].Value == "args" || val.Content[j].Value == "secrets" {
						removed++
					} else {
						kept = append(kept, val.Content[j], val.Content[j+1])
					}
				}
				val.Content = kept
			} else {
				removed += removeBuildArgsSecrets(val)
			}
		}
	}
	for _, child := range node.Content {
		if child.Kind == yaml.SequenceNode || child.Kind == yaml.DocumentNode {
			for _, item := range child.Content {
				removed += removeBuildArgsSecrets(item)
			}
		}
	}
	return removed
}
