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
	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
	repairui "github.com/postman/astro/apps/astro-cli/internal/tui/repair"
	spec "github.com/postman/astro/packages/astro-spec"
)

var repairCmd = &cobra.Command{
	Use:    "repair",
	Short:  "Check and repair project files against the template",
	Hidden: true,
	RunE:   runRepair,
}

func init() {
	rootCmd.AddCommand(repairCmd)
	repairCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Update all outdated files without prompting")
}

type repairFileCheck struct {
	path         string
	templatePath string
}

func runRepair(cmd *cobra.Command, args []string) error {
	specFile, _ := cmd.Flags().GetString("file")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath := filepath.Join(workingDir, specFile)
	astroSpec, specErr := spec.ParseSpec(specPath)

	var config scaffold.ScaffoldConfig
	var specMissing bool

	if specErr != nil {
		specMissing = true
		config = inferConfigFallback(workingDir)
		fmt.Println()
		fmt.Printf("%s!%s astroai.yml is missing or invalid — using inferred config (name: %s%s%s)\n",
			colorYellow, colorReset, colorBold, config.Name, colorReset)
	} else {
		config = configFromSpec(astroSpec)
	}

	paths, err := scaffold.GetTemplatePaths("ts")
	if err != nil {
		return err
	}

	type fileEntry struct {
		check          repairFileCheck
		label          string
		defaultSelected bool
	}

	entries := []fileEntry{
		{repairFileCheck{filepath.Join(workingDir, "Dockerfile"), paths.Dockerfile}, "Dockerfile", true},
		{repairFileCheck{filepath.Join(workingDir, "tsconfig.json"), paths.Tsconfig}, "tsconfig.json", true},
		{repairFileCheck{filepath.Join(workingDir, ".npmrc"), paths.Npmrc}, ".npmrc", true},
		{repairFileCheck{filepath.Join(workingDir, ".gitignore"), paths.Gitignore}, ".gitignore", true},
		{repairFileCheck{filepath.Join(workingDir, ".dockerignore"), paths.Dockerignore}, ".dockerignore", true},
		{repairFileCheck{filepath.Join(workingDir, "CLAUDE.md"), paths.LlmMd}, "CLAUDE.md", true},
		{repairFileCheck{filepath.Join(workingDir, "AGENTS.md"), paths.LlmMd}, "AGENTS.md", true},
		{repairFileCheck{filepath.Join(workingDir, "package.json"), paths.PackageJson}, "package.json", false},
		{repairFileCheck{filepath.Join(workingDir, "README.md"), paths.Readme}, "README.md", false},
	}

	if config.Ingestion != "none" {
		entries = append(entries, fileEntry{
			repairFileCheck{filepath.Join(workingDir, "Dockerfile.ingestion"), paths.DockerfileIngestion},
			"Dockerfile.ingestion",
			true,
		})
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
	processIssue := func(path, templatePath, relPath, action string) {
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
		if err := writeRepairFile(path, templatePath, config); err != nil {
			fmt.Printf("  %s✗%s failed to %s: %v\n\n", colorRed, colorReset, strings.ToLower(action), err)
		} else {
			fmt.Printf("  %s✓%s %sd\n\n", colorGreen, colorReset, action)
			fixed++
		}
	}

	// astroai.yml: only surfaced when missing, never compared to template
	if specMissing {
		relPath := filepath.Base(specPath)
		fmt.Printf("  %s✗%s %s %s(missing)%s\n", colorRed, colorReset, relPath, colorDim, colorReset)
		processIssue(specPath, paths.AstroYml, relPath, "Create")
	}

	for _, check := range checks {
		rendered, err := renderTemplateString(check.templatePath, config)
		if err != nil {
			continue
		}

		relPath, _ := filepath.Rel(workingDir, check.path)
		existing, readErr := os.ReadFile(check.path)

		switch {
		case os.IsNotExist(readErr):
			fmt.Printf("  %s✗%s %s %s(missing)%s\n", colorRed, colorReset, relPath, colorDim, colorReset)
			processIssue(check.path, check.templatePath, relPath, "Create")
		case readErr != nil:
			fmt.Printf("  %s!%s %s %s(unreadable)%s\n\n", colorYellow, colorReset, relPath, colorDim, colorReset)
		case string(existing) != rendered:
			fmt.Printf("  %s~%s %s %s(differs from template)%s\n", colorYellow, colorReset, relPath, colorDim, colorReset)
			printDiff(relPath, string(existing), rendered)
			fmt.Println()
			processIssue(check.path, check.templatePath, relPath, "Update")
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

	// Scan for deprecated @saswatds/astro-messaging references
	checkDeprecatedPackages(workingDir, reader, yesFlag)

	// Delete bun.lock so the updated .npmrc is picked up on next install
	bunLock := filepath.Join(workingDir, "bun.lock")
	if _, err := os.Stat(bunLock); err == nil {
		if removeErr := os.Remove(bunLock); removeErr != nil {
			fmt.Printf("%s!%s Failed to remove bun.lock: %v\n", colorYellow, colorReset, removeErr)
		} else {
			fmt.Printf("%s✓%s Removed bun.lock\n", colorGreen, colorReset)
		}
		fmt.Printf("%s!%s Run %sbun install%s to reinstall dependencies\n\n", colorYellow, colorReset, colorBold, colorReset)
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

func writeRepairFile(outputPath, templatePath string, config scaffold.ScaffoldConfig) error {
	rendered, err := renderTemplateString(templatePath, config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(rendered), 0644)
}

// inferConfigFallback builds a minimal ScaffoldConfig when astroai.yml is absent or unparseable.
// It tries to read the project name and description from package.json, falling back to the directory name.
func inferConfigFallback(workingDir string) scaffold.ScaffoldConfig {
	config := scaffold.DefaultConfig(filepath.Base(workingDir))

	pkgPath := filepath.Join(workingDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
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
		Description:     s.Meta.Description,
	}

	// Interfaces from dev section
	if s.Dev != nil {
		config.Interfaces = s.Dev.Interfaces
	}
	if len(config.Interfaces) == 0 {
		config.Interfaces = []string{"web"}
	}

	// Self-hosted model provider and model name
	for _, model := range s.Models {
		p := strings.ToLower(model.Provider)
		if p == "ollama" || p == "huggingface" {
			config.ModelProvider = model.Provider
			config.Model = model.Model
			break
		}
	}

	// Integrations: cloud model providers + explicit integration entries
	integrationSet := map[string]bool{}
	for _, model := range s.Models {
		p := strings.ToLower(model.Provider)
		if p == "anthropic" || p == "openai" {
			integrationSet[p] = true
		}
	}
	for name := range s.Integrations {
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

	// Ingestion trigger type
	config.Ingestion = "none"
	for _, ing := range s.Ingestion {
		config.Ingestion = ing.Trigger.Type
		break
	}

	return config
}

// checkDeprecatedPackages scans project files for @saswatds/astro-messaging and prompts to update.
func checkDeprecatedPackages(workingDir string, reader *bufio.Reader, yes bool) {
	const oldPkg = "@saswatds/astro-messaging"
	const newPkg = "@astromode-ai/astro-messaging"

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
		data, err := os.ReadFile(path)
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
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		newData := pattern.ReplaceAll(data, []byte(newPkg))
		if err := os.WriteFile(fullPath, newData, 0644); err != nil {
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
