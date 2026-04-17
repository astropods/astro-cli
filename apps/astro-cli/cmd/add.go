//go:build ignore

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/specwriter"
	"github.com/astropods/astro/apps/astro-cli/internal/tui/add"
	"github.com/astropods/astro/apps/astro-cli/internal/tui/credentials"
	spec "github.com/astropods/astro/packages/astro-spec"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a resource to astropods.yml",
	Long:  "Add a model, knowledge store, tool, or ingestion pipeline to astropods.yml interactively.",
}

var addModelCmd = &cobra.Command{
	Use:   "model <provider>",
	Short: "Add a model to astropods.yml",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAddModel,
}

var addKnowledgeCmd = &cobra.Command{
	Use:   "knowledge <provider>",
	Short: "Add a knowledge store to astropods.yml",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAddKnowledge,
}

var addToolCmd = &cobra.Command{
	Use:   "tool <provider>",
	Short: "Add a tool integration to astropods.yml",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAddIntegration,
}

var addIngestionCmd = &cobra.Command{
	Use:   "ingestion",
	Short: "Add an ingestion pipeline to astropods.yml",
	Args:  cobra.NoArgs,
	RunE:  runAddIngestion,
}

var addProviderCmd = &cobra.Command{
	Use:   "provider <name>",
	Short: "Add a custom provider to astropods.yml",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAddProvider,
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.AddCommand(addModelCmd, addKnowledgeCmd, addToolCmd, addIngestionCmd, addProviderCmd)
}

func runAddDomain(cmd *cobra.Command, domain, provider string) error {
	specPath, err := resolveSpecPathFromCwd(cmd)
	if err != nil {
		return err
	}
	sectionKey := domainToSection(domain)
	existing := specwriter.SectionNames(specPath, sectionKey)

	result, err := add.Run(domain, provider, existing)
	if err != nil {
		return err
	}

	if err := specwriter.AddEntry(specPath, sectionKey, result.Name, result.Entry); err != nil {
		return err
	}

	fmt.Printf("Added %s '%s' to %s\n", domain, result.Name, specPath)

	if creds := builtinCredentials(domain, provider); len(creds) > 0 {
		collectCredentials(specPath, provider, creds)
	} else if vars := customProviderCredentials(specPath, provider); len(vars) > 0 {
		collectCredentials(specPath, provider, vars)
	}
	return nil
}

func domainToSection(domain string) string {
	switch domain {
	case "model":
		return "models"
	case "knowledge":
		return "knowledge"
	case "tool":
		return "tools"
	case "ingestion":
		return "ingestion"
	}
	return domain
}

func runAddModel(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Usage: ast add model <provider>\n\nAvailable providers:\n  %s\n  (or a custom provider defined in providers:)\n", strings.Join(add.ValidModelProviders, "\n  "))
		return nil
	}
	provider := args[0]
	specPath, err := resolveSpecPathFromCwd(cmd)
	if err != nil {
		return err
	}
	customProviders := specwriter.SectionNames(specPath, "providers")
	if !slices.Contains(add.ValidModelProviders, provider) && !customProviders[provider] {
		return fmt.Errorf("invalid provider %q\n\nAvailable providers:\n  %s\n  (or a custom provider defined in providers:)", provider, strings.Join(add.ValidModelProviders, "\n  "))
	}
	return runAddDomain(cmd, "model", provider)
}

func runAddKnowledge(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Usage: ast add knowledge <provider>\n\nAvailable providers:\n  %s\n  (or a custom provider defined in providers:)\n", strings.Join(add.ValidKnowledgeProviders, "\n  "))
		return nil
	}
	provider := args[0]
	specPath, err := resolveSpecPathFromCwd(cmd)
	if err != nil {
		return err
	}
	customProviders := specwriter.SectionNames(specPath, "providers")
	if !slices.Contains(add.ValidKnowledgeProviders, provider) && !customProviders[provider] {
		return fmt.Errorf("invalid provider %q\n\nAvailable providers:\n  %s\n  (or a custom provider defined in providers:)", provider, strings.Join(add.ValidKnowledgeProviders, "\n  "))
	}
	return runAddDomain(cmd, "knowledge", provider)
}

func runAddIntegration(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Usage: ast add tool <provider>\n\nAvailable providers:\n  %s\n  (or a custom provider defined in providers:)\n", strings.Join(add.ValidIntegrationProviders, "\n  "))
		return nil
	}
	provider := args[0]
	specPath, err := resolveSpecPathFromCwd(cmd)
	if err != nil {
		return err
	}
	customProviders := specwriter.SectionNames(specPath, "providers")
	if !slices.Contains(add.ValidIntegrationProviders, provider) && !customProviders[provider] {
		return fmt.Errorf("invalid provider %q\n\nAvailable providers:\n  %s\n  (or a custom provider defined in providers:)", provider, strings.Join(add.ValidIntegrationProviders, "\n  "))
	}
	return runAddDomain(cmd, "tool", provider)
}

func runAddIngestion(cmd *cobra.Command, _ []string) error {
	return runAddDomain(cmd, "ingestion", "")
}

func runAddProvider(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Usage: ast add provider <name>\n\nProvide a name for the custom provider (e.g. ast add provider my-llm).\n")
		return nil
	}
	name := args[0]
	specPath, err := resolveSpecPathFromCwd(cmd)
	if err != nil {
		return err
	}

	result, err := add.Run("provider", name, specwriter.SectionNames(specPath, "providers"))
	if err != nil {
		return err
	}

	if err := specwriter.AddEntry(specPath, "providers", name, result.Entry); err != nil {
		return err
	}

	fmt.Printf("Added provider '%s' to %s\n", name, specPath)
	fmt.Printf("Use '%s add model/knowledge/tool %s' to activate it in a domain.\n", binaryName, name)
	return nil
}

// customProviderCredentials returns the variables for a custom provider as ProviderVars.
// Keys follow §8.1: {UPPER(provider)}_{varName} (simple non-duplicate form used at add time).
// Returns nil if the provider is not a custom provider.
func customProviderCredentials(specPath, providerName string) []add.ProviderVar {
	s, err := spec.ParseFile(specPath)
	if err != nil {
		return nil
	}
	cp, ok := s.Providers[providerName]
	if !ok {
		return nil
	}
	prefix := spec.SanitizeEnvName(providerName)
	var vars []add.ProviderVar
	for _, v := range cp.Variables {
		vars = append(vars, add.ProviderVar{Name: prefix + "_" + v.Name, Secret: v.Secret})
	}
	return vars
}

// builtinCredentials returns the credentials required by a builtin cloud
// provider, expressed as ProviderVars with the full env var name (e.g. GITHUB_TOKEN).
// Returns nil for self-hosted providers and ingestion.
func builtinCredentials(domain, provider string) []add.ProviderVar {
	section := domainToSection(domain)
	p, ok := spec.LookupBuiltin(section, provider)
	if !ok || !p.Cloud {
		return nil
	}
	vars := make([]add.ProviderVar, len(p.Credentials))
	prefix := strings.ToUpper(provider)
	for i, c := range p.Credentials {
		vars[i] = add.ProviderVar{Name: prefix + "_" + c.Suffix, Secret: true}
	}
	return vars
}

// collectCredentials launches the credentials TUI for the given vars and
// appends any entered values to the .env file next to the spec.
func collectCredentials(specPath, providerName string, vars []add.ProviderVar) {
	creds := make([]credentials.Credential, len(vars))
	for i, v := range vars {
		creds[i] = credentials.Credential{Name: v.Name, Secret: v.Secret}
	}

	values, err := credentials.Run(providerName, creds)
	if err != nil || len(values) == 0 {
		return
	}

	envPath := filepath.Join(filepath.Dir(specPath), ".env")

	if err := writeEnv(envPath, providerName, creds, values); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", envPath, err)
		return
	}

	fmt.Printf("Credentials written to %s\n", envPath)
}

// quoteDotEnvValue wraps v in double quotes and escapes characters that would
// otherwise malform a .env file: backslashes, double quotes, and newlines.
func quoteDotEnvValue(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, "\r", `\r`)
	return `"` + v + `"`
}

// writeEnv writes credential values into the .env file at path.
// Existing keys are overwritten in-place with a warning; new keys are appended.
func writeEnv(path, providerName string, creds []credentials.Credential, values credentials.Result) error {
	// Read existing content (file may not exist yet).
	existing, _ := os.ReadFile(path) //nolint:gosec
	lines := strings.Split(string(existing), "\n")
	// Trim a single trailing empty element produced by a final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	replaced := map[string]bool{}

	// Overwrite any keys that already appear in the file.
	for i, line := range lines {
		k, _, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		k = strings.TrimSpace(k)
		if v, want := values[k]; want {
			lines[i] = k + "=" + quoteDotEnvValue(v)
			replaced[k] = true
			fmt.Printf("warning: overwrote existing %s in %s\n", k, path)
		}
	}

	// Append keys that weren't already present.
	var appended []string
	for _, c := range creds {
		if replaced[c.Name] {
			continue
		}
		if v, ok := values[c.Name]; ok {
			appended = append(appended, c.Name+"="+quoteDotEnvValue(v))
		}
	}
	if len(appended) > 0 {
		lines = append(lines, "", "# "+providerName+" credentials")
		lines = append(lines, appended...)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600) //nolint:gosec
}
