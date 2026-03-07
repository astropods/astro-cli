package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	spec "github.com/astropods/astro/packages/astro-spec"
)

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain the agent project based on its spec",
	Long: `Parse the astropods.yml spec and display a human-readable explanation
of the agent project: its components, what env vars each component
injects into the agent, and what credentials and inputs are required.

Example:
  ast explain
  ast explain -f custom-spec.yml`,
	RunE: runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func runExplain(cmd *cobra.Command, args []string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPath(cmd, workingDir)
	if err != nil {
		return err
	}

	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	specDir := filepath.Dir(specPath)
	return printExplain(astroSpec, specDir, workingDir)
}

// resolvePath takes a build context path (relative to the spec file) and returns
// a clean path relative to the working directory.
func resolvePath(contextPath, specDir, workingDir string) string {
	if filepath.IsAbs(contextPath) {
		return contextPath
	}
	abs := filepath.Join(specDir, contextPath)
	rel, err := filepath.Rel(workingDir, abs)
	if err != nil {
		return abs
	}
	return rel
}

func printExplain(astroSpec *spec.AstroSpec, specDir, workingDir string) error {
	fmt.Println()

	// ── Header ──────────────────────────────────────────────────────────────
	fmt.Printf("%s%s%s%s\n", colorBold, colorGreen, astroSpec.Name, colorReset)
	if astroSpec.Meta.Description != "" {
		fmt.Printf("%s%s%s\n", colorDim, astroSpec.Meta.Description, colorReset)
	}
	if len(astroSpec.Meta.Tags) > 0 {
		fmt.Printf("%stags: %v%s\n", colorDim, astroSpec.Meta.Tags, colorReset)
	}
	fmt.Println()

	// ── Container ───────────────────────────────────────────────────────────
	sectionHeader("Container", "main agent process")
	if astroSpec.Agent.Build != nil {
		resolved := resolvePath(astroSpec.Agent.Build.Context, specDir, workingDir)
		detail := resolved
		if astroSpec.Agent.Build.Dockerfile != "" {
			detail += " / " + astroSpec.Agent.Build.Dockerfile
		}
		fmt.Printf("  build:  %s%s%s\n", colorYellow, detail, colorReset)
	} else if astroSpec.Agent.Image != "" {
		fmt.Printf("  image:  %s%s%s\n", colorYellow, astroSpec.Agent.Image, colorReset)
	}
	if astroSpec.Agent.Distributed {
		fmt.Printf("  multi-replica: yes\n")
	}
	printInputList(astroSpec.Agent.Inputs, "  ")
	fmt.Println()

	// ── Models ──────────────────────────────────────────────────────────────
	if len(astroSpec.Models) > 0 {
		sectionHeader("Models", "AI inference servers")
		for _, name := range sortedKeys(astroSpec.Models) {
			model := astroSpec.Models[name]
			printModelEntry(name, model, astroSpec, specDir, workingDir)
		}
		fmt.Println()
	}

	// ── Knowledge ───────────────────────────────────────────────────────────
	if len(astroSpec.Knowledge) > 0 {
		sectionHeader("Knowledge", "data stores for memory and context")
		for _, name := range sortedKeys(astroSpec.Knowledge) {
			k := astroSpec.Knowledge[name]
			printKnowledgeEntry(name, k, astroSpec, specDir, workingDir)
		}
		fmt.Println()
	}

	// ── Tools ───────────────────────────────────────────────────────────────
	if len(astroSpec.Tools) > 0 {
		sectionHeader("Tools", "callable capabilities")
		for _, name := range sortedKeys(astroSpec.Tools) {
			tool := astroSpec.Tools[name]
			printToolEntry(name, tool, astroSpec, specDir, workingDir)
		}
		fmt.Println()
	}

	// ── Providers ───────────────────────────────────────────────────────────
	if len(astroSpec.Providers) > 0 {
		sectionHeader("Providers", "custom provider templates")
		for _, name := range sortedKeys(astroSpec.Providers) {
			provider := astroSpec.Providers[name]
			fmt.Printf("\n  %s%s%s  %sscope: %s%s\n",
				colorCyan, name, colorReset,
				colorDim, strings.Join(provider.Scope, ", "), colorReset)
			for _, v := range provider.Variables {
				printVariableLine(v, "    ")
			}
		}
		fmt.Println()
	}

	// ── Top-level Inputs ────────────────────────────────────────────────────
	if len(astroSpec.Inputs) > 0 {
		sectionHeader("Inputs", "injected into all containers at deploy")
		for _, name := range sortedKeys(astroSpec.Inputs) {
			inp := astroSpec.Inputs[name]
			printVariableLine(inp, "  ")
		}
		fmt.Println()
	}

	// ── Ingestion ───────────────────────────────────────────────────────────
	if len(astroSpec.Ingestion) > 0 {
		sectionHeader("Ingestion", "background data pipeline jobs")

		// All ingestion containers inherit the agent environment.
		// Show the shared consumes list once at section level, not per entry.
		var sharedConsumes []string
		for k := range spec.AgentConnectionKeys(astroSpec, nil) {
			sharedConsumes = append(sharedConsumes, k)
		}
		for k := range spec.AllCredentialKeys(astroSpec) {
			sharedConsumes = append(sharedConsumes, k)
		}
		for k := range astroSpec.Inputs {
			sharedConsumes = append(sharedConsumes, k)
		}
		sort.Strings(sharedConsumes)
		if len(sharedConsumes) > 0 {
			printKeyList("  all jobs consume:", "                   ", sharedConsumes, 80)
		}

		for _, name := range sortedKeys(astroSpec.Ingestion) {
			ing := astroSpec.Ingestion[name]
			printIngestionEntry(name, ing, astroSpec, specDir, workingDir)
		}
		fmt.Println()
	}

	// ── Dev Interfaces ──────────────────────────────────────────────────────
	if astroSpec.Dev != nil && len(astroSpec.Dev.Interfaces) > 0 {
		sectionHeader("Dev Interfaces", "local messaging channels")
		for _, name := range astroSpec.Dev.Interfaces {
			envs := getInterfaceEnvVars(name)
			fmt.Printf("  %s%s%s", colorCyan, name, colorReset)
			if len(envs) > 0 {
				fmt.Printf("  %s→ sidecar needs: %s%s",
					colorDim, strings.Join(envs, ", "), colorReset)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// ── Agent receives ──────────────────────────────────────────────────────
	printAgentReceives(astroSpec)

	// ── Warnings ────────────────────────────────────────────────────────────
	warnings := collectWarnings(astroSpec)
	if len(warnings) > 0 {
		fmt.Printf("%s%sWarnings%s\n\n", colorBold, colorYellow, colorReset)
		for _, w := range warnings {
			fmt.Printf("  %s!%s %s\n", colorYellow, colorReset, w)
		}
		fmt.Println()
	}

	return nil
}

// ── Component printers ───────────────────────────────────────────────────────

func printModelEntry(name string, model spec.Model, s *spec.AstroSpec, specDir, workingDir string) {
	mc := model.ResolvedContainer()

	// Title line
	fmt.Printf("\n  %s%s%s", colorCyan, name, colorReset)
	if model.Provider != "" {
		provType := providerKind(model.Provider, "models", s)
		fmt.Printf("  %s%s  ·  %s%s", colorDim, model.Provider, provType, colorReset)
	}
	fmt.Println()

	if model.DeploysContainer(s.Providers) {
		if model.Container != nil && model.Container.Build != nil {
			resolved := resolvePath(model.Container.Build.Context, specDir, workingDir)
			fmt.Printf("    build:  %s%s%s\n", colorYellow, resolved, colorReset)
		} else if mc.Image != "" {
			fmt.Printf("    image:  %s%s%s\n", colorDim, mc.Image, colorReset)
		}
		if mc.Port > 0 {
			fmt.Printf("    port:   %d\n", mc.Port)
		}
		if mc.HasGPU() {
			fmt.Printf("    gpu:    %syes%s\n", colorGreen, colorReset)
		}
		if model.Model != "" {
			fmt.Printf("    model:  %s%s%s\n", colorDim, model.Model, colorReset)
		}
	}

	// Output: what this component injects into the agent env
	printComponentOutput(s, "models", name, model.Provider, model.Model, s)

	// Component-specific inputs
	printInputList(model.Inputs, "    ")
}

func printKnowledgeEntry(name string, k spec.Knowledge, s *spec.AstroSpec, specDir, workingDir string) {
	kc := k.ResolvedContainer()

	fmt.Printf("\n  %s%s%s", colorCyan, name, colorReset)
	if k.Provider != "" {
		provType := providerKind(k.Provider, "knowledge", s)
		fmt.Printf("  %s%s  ·  %s%s", colorDim, k.Provider, provType, colorReset)
	}
	if kc.Persistent {
		fmt.Printf("  %spersistent%s", colorGreen, colorReset)
	}
	fmt.Println()

	if kc.Build != nil {
		resolved := resolvePath(kc.Build.Context, specDir, workingDir)
		fmt.Printf("    build:  %s%s%s\n", colorYellow, resolved, colorReset)
	} else if kc.Image != "" {
		fmt.Printf("    image:  %s%s%s\n", colorDim, kc.Image, colorReset)
	}
	if kc.Port > 0 {
		fmt.Printf("    port:   %d\n", kc.Port)
	}

	printComponentOutput(s, "knowledge", name, k.Provider, "", s)
	printInputList(k.Inputs, "    ")
}

func printToolEntry(name string, tool spec.Tool, s *spec.AstroSpec, specDir, workingDir string) {
	fmt.Printf("\n  %s%s%s", colorCyan, name, colorReset)
	if tool.Provider != "" {
		provType := providerKind(tool.Provider, "tools", s)
		fmt.Printf("  %s%s  ·  %s%s", colorDim, tool.Provider, provType, colorReset)
	} else {
		fmt.Printf("  %s(container)%s", colorDim, colorReset)
	}
	fmt.Println()

	if tool.Container != nil {
		if tool.Container.Build != nil {
			resolved := resolvePath(tool.Container.Build.Context, specDir, workingDir)
			fmt.Printf("    build:  %s%s%s\n", colorYellow, resolved, colorReset)
		} else if tool.Container.Image != "" {
			fmt.Printf("    image:  %s%s%s\n", colorDim, tool.Container.Image, colorReset)
		}
		if tool.Container.Port > 0 {
			fmt.Printf("    port:   %d\n", tool.Container.Port)
		}
	}

	printComponentOutput(s, "tools", name, tool.Provider, "", s)
	printInputList(tool.Inputs, "    ")
}

func printIngestionEntry(name string, ing spec.Ingestion, s *spec.AstroSpec, specDir, workingDir string) {
	// Title: name + trigger type (+ cron schedule if set in dev)
	fmt.Printf("\n  %s%s%s  %s%s%s", colorCyan, name, colorReset, colorDim, ing.Trigger.Type, colorReset)
	if s.Dev != nil {
		if sched := s.Dev.Schedules[name]; sched != "" {
			fmt.Printf("  %s%s%s", colorDim, sched, colorReset)
		}
	}
	fmt.Println()

	// Image / build
	if ing.Container.Build != nil {
		resolved := resolvePath(ing.Container.Build.Context, specDir, workingDir)
		detail := resolved
		if ing.Container.Build.Dockerfile != "" {
			detail += " / " + ing.Container.Build.Dockerfile
		}
		fmt.Printf("    build:  %s%s%s\n", colorYellow, detail, colorReset)
	} else if ing.Container.Image != "" {
		fmt.Printf("    image:  %s%s%s\n", colorDim, ing.Container.Image, colorReset)
	}

	// Webhook trigger requires a port
	if ing.Trigger.Type == "webhook" {
		if ing.Container.Port > 0 {
			fmt.Printf("    port:   %d\n", ing.Container.Port)
		} else {
			fmt.Printf("    port:   %s(none — webhook trigger should declare a port)%s\n", colorYellow, colorReset)
		}
	}

	// Ingestion-specific inputs (the shared consumes list is shown once at section level)
	printInputList(ing.Inputs, "    ")
}

// printComponentOutput shows what a component produces into the agent's environment.
// All component types use a single "produces:" label — the mechanism (connection
// wiring, credential injection, provider variables) is irrelevant to the consumer.
func printComponentOutput(_ *spec.AstroSpec, section, name, provider, modelName string, full *spec.AstroSpec) {
	var keys []string

	if provider != "" {
		if isSectionCloudProvider(provider, section) {
			for k, meta := range spec.CloudCredentialKeys(full) {
				if meta.Provider == strings.ToLower(provider) {
					keys = append(keys, k)
				}
			}
		} else if cp, ok := full.Providers[provider]; ok {
			for _, v := range cp.Variables {
				keys = append(keys, v.Name)
			}
		}
	}

	if len(keys) == 0 {
		// Self-hosted provider or container mode: connection env vars.
		keys = spec.AgentKeysForComponent(full, section, name)
		// MODEL key carries a static value so the sentinel approach misses it — add explicitly.
		if section == "models" && modelName != "" && provider != "" {
			p := spec.GetModelProvider(provider)
			if p.EnvPrefix != "" {
				provCount := 0
				for _, m := range full.Models {
					if m.IsProviderMode() && strings.EqualFold(m.Provider, provider) {
						provCount++
					}
				}
				modelKey := p.EnvPrefix + "_MODEL"
				if provCount > 1 {
					modelKey = p.EnvPrefix + "_" + spec.SanitizeEnvName(name) + "_MODEL"
				}
				keys = appendIfMissing(keys, modelKey)
			}
		}
	}

	sort.Strings(keys)
	if len(keys) > 0 {
		printKeyList("    produces:", "                ", keys, 80)
	}
}

// ── Agent receives summary ────────────────────────────────────────────────────

func printAgentReceives(s *spec.AstroSpec) {
	// Connection keys (from self-hosted + container components)
	connEnv := spec.AgentConnectionKeys(s, nil)
	// Group by component for display
	var connKeys []string
	for k := range connEnv {
		connKeys = append(connKeys, k)
	}
	sort.Strings(connKeys)

	// Credential keys
	credMap := spec.AllCredentialKeys(s)
	var credRequired, credOptional []string
	for k, meta := range credMap {
		if meta.Optional {
			credOptional = append(credOptional, k)
		} else {
			credRequired = append(credRequired, k)
		}
	}
	sort.Strings(credRequired)
	sort.Strings(credOptional)

	// Non-secret custom provider variables (injected as plain env vars)
	var plainProviderVars []string
	for _, cp := range referencedCustomProviders(s) {
		for _, v := range cp.Variables {
			if !v.Secret {
				plainProviderVars = append(plainProviderVars, v.Name)
			}
		}
	}
	sort.Strings(plainProviderVars)

	// Inputs
	var topInputNames []string
	for k := range s.Inputs {
		topInputNames = append(topInputNames, k)
	}
	sort.Strings(topInputNames)
	var agentInputNames []string
	for _, inp := range s.Agent.Inputs {
		agentInputNames = append(agentInputNames, inp.Name)
	}

	// Only print the section if there's something to show
	if len(connKeys) == 0 && len(credRequired) == 0 && len(credOptional) == 0 &&
		len(plainProviderVars) == 0 && len(topInputNames) == 0 && len(agentInputNames) == 0 {
		return
	}

	sectionHeader("Agent consumes", "complete env var list visible to agent code")

	// Flatten everything into one sorted list grouped by source type.
	if len(connKeys) > 0 {
		printKeyList("  from producers:", "                  ", connKeys, 80)
	}

	if len(credRequired) > 0 || len(credOptional) > 0 || len(plainProviderVars) > 0 {
		allProviderKeys := append(append(credRequired, credOptional...), plainProviderVars...)
		sort.Strings(allProviderKeys)
		printKeyList("  from providers:", "                  ", allProviderKeys, 80)
	}

	if len(topInputNames) > 0 || len(agentInputNames) > 0 {
		fmt.Printf("  %sfrom inputs:%s\n", colorDim, colorReset)
		for _, name := range topInputNames {
			inp := s.Inputs[name]
			printInputSummaryLine(inp, "    ", "(all containers)")
		}
		for _, inp := range s.Agent.Inputs {
			printInputSummaryLine(inp, "    ", "(agent only)")
		}
	}
	fmt.Println()
}

// ── Warnings ─────────────────────────────────────────────────────────────────

// collectWarnings checks for overlapping env vars, duplicate ports, and shadows.
// Uses the resolver for accurate connection key computation.
func collectWarnings(s *spec.AstroSpec) []string {
	var warnings []string

	// Build the authoritative set of auto-injected agent env vars using the resolver.
	autoEnv := make(map[string]string) // key → source description

	// Connection keys (self-hosted + container mode) and credential keys (cloud +
	// custom provider secrets) — combined by AllAgentAutoEnvKeys.
	for key, meta := range spec.AllAgentAutoEnvKeys(s) {
		var desc string
		switch meta.Source {
		case "connection":
			desc = "component connection wiring"
		case "credential":
			desc = fmt.Sprintf("cloud provider %s%s%s", colorCyan, meta.Provider, colorReset)
		}
		if prev, ok := autoEnv[key]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"Env var %s%s%s is claimed by both %s and %s.",
				colorBold, key, colorReset, prev, desc))
		}
		autoEnv[key] = desc
	}

	// Custom provider non-secret variables (secrets already covered above).
	for provName, cp := range referencedCustomProviders(s) {
		for _, v := range cp.Variables {
			if v.Secret {
				continue // already in AllAgentAutoEnvKeys via AllCredentialKeys
			}
			desc := fmt.Sprintf("provider %s%s%s", colorCyan, provName, colorReset)
			if prev, ok := autoEnv[v.Name]; ok {
				warnings = append(warnings, fmt.Sprintf(
					"Env var %s%s%s is claimed by both %s and %s.",
					colorBold, v.Name, colorReset, prev, desc))
			}
			autoEnv[v.Name] = desc
		}
	}

	// System-reserved keys
	reserved := map[string]string{
		"GRPC_SERVER_ADDR":            "interfaces (messaging sidecar)",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "observability collector",
		"ASTRO_AGENT_NAME":            "system",
		"ASTRO_AGENT_BUILD":           "system",
	}
	for key, src := range reserved {
		autoEnv[key] = src
	}

	// Check user-provided container.environment fields for shadows
	// Only check explicitly user-supplied environment fields (not provider defaults).
	checkEnv := func(env map[string]string, label string) {
		for key := range env {
			if src, ok := autoEnv[key]; ok {
				warnings = append(warnings, fmt.Sprintf(
					"%s sets env var %s%s%s which shadows the value auto-injected by %s.",
					label, colorBold, key, colorReset, src))
			}
		}
	}
	for name, m := range s.Models {
		if m.Container != nil {
			checkEnv(m.Container.Environment,
				fmt.Sprintf("Model %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, k := range s.Knowledge {
		if k.Container != nil {
			checkEnv(k.Container.Environment,
				fmt.Sprintf("Knowledge %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, t := range s.Tools {
		if t.Container != nil {
			checkEnv(t.Container.Environment,
				fmt.Sprintf("Tool %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, ing := range s.Ingestion {
		checkEnv(ing.Container.Environment,
			fmt.Sprintf("Ingestion %s%s%s", colorCyan, name, colorReset))
	}

	// Duplicate ports
	portUsers := make(map[int][]string)
	for name, m := range s.Models {
		if !m.DeploysContainer(s.Providers) {
			continue
		}
		if mc := m.ResolvedContainer(); mc.Port > 0 {
			portUsers[mc.Port] = append(portUsers[mc.Port],
				fmt.Sprintf("model %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, k := range s.Knowledge {
		if k.IsProviderMode() && spec.IsCloudKnowledgeProvider(k.Provider) {
			continue // cloud providers have no container
		}
		if kc := k.ResolvedContainer(); kc.Port > 0 {
			portUsers[kc.Port] = append(portUsers[kc.Port],
				fmt.Sprintf("knowledge %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, t := range s.Tools {
		if t.Container != nil && t.Container.Port > 0 {
			portUsers[t.Container.Port] = append(portUsers[t.Container.Port],
				fmt.Sprintf("tool %s%s%s", colorCyan, name, colorReset))
		}
	}
	for port, users := range portUsers {
		if len(users) > 1 {
			warnings = append(warnings, fmt.Sprintf(
				"Port %s%d%s is used by multiple components: %s.",
				colorBold, port, colorReset, strings.Join(users, ", ")))
		}
	}

	return warnings
}

// ── Input / variable display helpers ─────────────────────────────────────────

// printInputList prints a list of component-specific inputs with a label.
func printInputList(inputs []spec.Input, indent string) {
	if len(inputs) == 0 {
		return
	}
	fmt.Printf("%s%sinputs (provided at deploy):%s\n", indent, colorDim, colorReset)
	for _, inp := range inputs {
		printVariableLine(inp, indent+"  ")
	}
}

// printVariableLine prints a single input/variable in a compact format.
func printVariableLine(inp spec.Input, indent string) {
	parts := []string{inp.Datatype}
	if inp.Secret {
		parts = append(parts, "secret")
	}
	if inp.Optional {
		parts = append(parts, "optional")
	} else {
		parts = append(parts, "required")
	}
	if inp.DisplayAs == "select" && len(inp.Options) > 0 {
		parts = append(parts, "select: "+strings.Join(inp.Options, "|"))
	}

	meta := strings.Join(parts, "  ·  ")
	if inp.Default != "" {
		fmt.Printf("%s%s%s%s  %s=%s  %s[%s]%s\n",
			indent,
			colorYellow, inp.Name, colorReset,
			colorDim, inp.Default, colorReset,
			colorDim, meta+colorReset)
	} else {
		fmt.Printf("%s%s%s%s  %s%s%s\n",
			indent,
			colorYellow, inp.Name, colorReset,
			colorDim, meta, colorReset)
	}
	if inp.Description != "" {
		fmt.Printf("%s  %s%s%s\n", indent, colorDim, inp.Description, colorReset)
	}
}

// printInputSummaryLine prints a compact one-liner for the "Agent receives → inputs" section.
func printInputSummaryLine(inp spec.Input, indent, scope string) {
	optStr := ""
	if inp.Optional {
		optStr = "  optional"
	}
	defaultStr := ""
	if inp.Default != "" {
		defaultStr = fmt.Sprintf("  =%s", inp.Default)
	}
	fmt.Printf("%s%s%s%s%s%s  %s%s%s\n",
		indent,
		colorYellow, inp.Name, colorReset,
		defaultStr,
		optStr,
		colorDim, scope, colorReset)
}

// ── Misc helpers ─────────────────────────────────────────────────────────────

func sectionHeader(title, subtitle string) {
	fmt.Printf("%s%s%s  %s%s%s\n", colorBold, colorBlue, title, colorDim, subtitle, colorReset)
}

// providerKind returns a display label for the kind of provider.
func providerKind(provider, section string, s *spec.AstroSpec) string {
	if _, isCustom := s.Providers[provider]; isCustom {
		return "custom provider"
	}
	if p, ok := spec.LookupBuiltin(section, provider); ok {
		if p.Cloud {
			return "cloud"
		}
		return "self-hosted"
	}
	return ""
}

// isSectionCloudProvider returns true if provider is a built-in cloud provider for the given section.
func isSectionCloudProvider(provider, section string) bool {
	p, ok := spec.LookupBuiltin(section, provider)
	return ok && p.Cloud
}

// referencedCustomProviders returns custom providers actually referenced by components.
func referencedCustomProviders(s *spec.AstroSpec) map[string]spec.CustomProvider {
	out := make(map[string]spec.CustomProvider)
	for _, m := range s.Models {
		if m.IsProviderMode() {
			if cp, ok := s.Providers[m.Provider]; ok {
				out[m.Provider] = cp
			}
		}
	}
	for _, k := range s.Knowledge {
		if k.IsProviderMode() {
			if cp, ok := s.Providers[k.Provider]; ok {
				out[k.Provider] = cp
			}
		}
	}
	for _, t := range s.Tools {
		if t.IsProviderMode() {
			if cp, ok := s.Providers[t.Provider]; ok {
				out[t.Provider] = cp
			}
		}
	}
	return out
}

// sortedKeys returns alphabetically sorted keys of any map.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// printKeyList prints a labelled list of env var keys, wrapping at maxWidth columns.
// label is the prefix (including indent and trailing colon).
// indent is the continuation indent for wrapped lines — should align with where
// keys start on the first line.
func printKeyList(label, indent string, keys []string, maxWidth int) {
	if len(keys) == 0 {
		return
	}
	fmt.Printf("%s%s%s  ", colorDim, label, colorReset)
	col := len(label) + 2 // visible columns used so far (no ANSI)
	for i, k := range keys {
		// ", KEY" width or just "KEY" for the first
		needed := len(k)
		if i > 0 {
			needed += 2 // ", "
		}
		if i > 0 && col+needed > maxWidth {
			fmt.Printf("\n%s", indent)
			col = len(indent)
			fmt.Printf("%s%s%s", colorYellow, k, colorReset)
			col += len(k)
		} else {
			if i > 0 {
				fmt.Printf(", ")
				col += 2
			}
			fmt.Printf("%s%s%s", colorYellow, k, colorReset)
			col += len(k)
		}
	}
	fmt.Println()
}

func appendIfMissing(slice []string, item string) []string {
	for _, v := range slice {
		if v == item {
			return slice
		}
	}
	return append(slice, item)
}

// getInterfaceEnvVars returns the env vars consumed by a given messaging interface sidecar.
func getInterfaceEnvVars(name string) []string {
	switch strings.ToLower(name) {
	case "slack":
		return []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"}
	case "web":
		return []string{"WEB_LISTEN_ADDR"}
	default:
		return nil
	}
}
