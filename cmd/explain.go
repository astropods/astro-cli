package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	spec "github.com/postman/astro/packages/astro-spec"
)

var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain the agent project based on its spec",
	Long: `Parse the astro.yml spec and display a human-readable explanation
of the agent project: its components, how they connect, and what
gets built vs. pre-built.

Example:
  ast explain
  ast explain -f custom-spec.yml`,
	RunE: runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

func runExplain(cmd *cobra.Command, args []string) error {
	specFile, _ := cmd.Flags().GetString("file")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath := filepath.Join(workingDir, specFile)
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

	// Agent overview
	fmt.Printf("%s%s%s%s\n", colorBold, colorGreen, astroSpec.Name, colorReset)
	if astroSpec.Meta.Description != "" {
		fmt.Printf("%s%s%s\n", colorDim, astroSpec.Meta.Description, colorReset)
	}
	if len(astroSpec.Meta.Tags) > 0 {
		fmt.Printf("%s%v%s\n", colorDim, astroSpec.Meta.Tags, colorReset)
	}
	fmt.Println()

	// Container
	fmt.Printf("%s%sContainer%s  %sThe main runtime for the agent process%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
	if astroSpec.Agent.Build != nil {
		resolved := resolvePath(astroSpec.Agent.Build.Context, specDir, workingDir)
		fmt.Printf("  Build context: %s%s%s", colorYellow, resolved, colorReset)
		if astroSpec.Agent.Build.Dockerfile != "" {
			fmt.Printf(" (Dockerfile: %s%s%s)", colorYellow, astroSpec.Agent.Build.Dockerfile, colorReset)
		}
		fmt.Println()
	} else if astroSpec.Agent.Image != "" {
		fmt.Printf("  Image: %s%s%s\n", colorYellow, astroSpec.Agent.Image, colorReset)
	}
	fmt.Println()

	// Models
	if len(astroSpec.Models) > 0 {
		fmt.Printf("%s%sModels%s  %sSelf-hosted inference servers deployed as sidecars%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
		for name, model := range astroSpec.Models {
			fmt.Printf("\n  %s%s%s", colorCyan, name, colorReset)
			if model.Provider != "" {
				fmt.Printf(" %s(%s)%s", colorDim, model.Provider, colorReset)
			}
			fmt.Println()
			mc := model.ResolvedContainer()
			if model.Container != nil && model.Container.Build != nil {
				resolved := resolvePath(model.Container.Build.Context, specDir, workingDir)
				fmt.Printf("    Build context: %s%s%s\n", colorYellow, resolved, colorReset)
			} else if mc.Image != "" {
				fmt.Printf("    Image: %s%s%s\n", colorDim, mc.Image, colorReset)
			}
			if mc.Port > 0 {
				fmt.Printf("    Port: %d\n", mc.Port)
			}
			envKey := strings.ToUpper(name) + "_HOST"
			fmt.Printf("    Env: %s%s%s\n", colorDim, envKey, colorReset)
		}
		fmt.Println()
	}

	// Knowledge
	if len(astroSpec.Knowledge) > 0 {
		fmt.Printf("%s%sKnowledge%s  %sData stores that give the agent memory and context%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
		for name, k := range astroSpec.Knowledge {
			fmt.Printf("\n  %s%s%s", colorCyan, name, colorReset)
			if k.Provider != "" {
				fmt.Printf(" %s(%s)%s", colorDim, k.Provider, colorReset)
			}
			fmt.Println()
			kc := k.ResolvedContainer()
			if kc.Persistent {
				fmt.Printf("    Persistent: %s%syes%s\n", colorBold, colorGreen, colorReset)
			}
			if kc.Build != nil {
				resolved := resolvePath(kc.Build.Context, specDir, workingDir)
				fmt.Printf("    Build context: %s%s%s\n", colorYellow, resolved, colorReset)
			} else if kc.Image != "" {
				fmt.Printf("    Image: %s%s%s\n", colorDim, kc.Image, colorReset)
			}
			if k.Provider != "" {
				prov := spec.GetProvider(k.Provider)
				if prov.EnvPrefix != "" {
					fmt.Printf("    Env: %s%s_HOST%s, %s%s_PORT%s\n",
						colorDim, prov.EnvPrefix, colorReset,
						colorDim, prov.EnvPrefix, colorReset)
				}
			}
		}
		fmt.Println()
	}

	// Tools
	if len(astroSpec.Tools) > 0 {
		fmt.Printf("%s%sTools%s  %sCallable capabilities that extend what the agent can do%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
		for name, tool := range astroSpec.Tools {
			fmt.Printf("\n  %s%s%s", colorCyan, name, colorReset)
			if tool.Container != nil {
				fmt.Println(" (sidecar)")
				if tool.Container.Build != nil {
					resolved := resolvePath(tool.Container.Build.Context, specDir, workingDir)
					fmt.Printf("    Build context: %s%s%s\n", colorYellow, resolved, colorReset)
				} else if tool.Container.Image != "" {
					fmt.Printf("    Image: %s%s%s\n", colorDim, tool.Container.Image, colorReset)
				}
				if tool.Container.Port > 0 {
					fmt.Printf("    Port: %d\n", tool.Container.Port)
				}
			} else {
				fmt.Println(" (in-process)")
			}
		}
		fmt.Println()
	}

	// Integrations
	if len(astroSpec.Integrations) > 0 {
		fmt.Printf("%s%sIntegrations%s  %sThird-party APIs accessed via credentials at runtime%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
		for name, integration := range astroSpec.Integrations {
			fmt.Printf("\n  %s%s%s\n", colorCyan, name, colorReset)

			if len(integration.Credentials) > 0 {
				var keys []string
				for _, cc := range integration.Credentials {
					keys = append(keys, strings.ToUpper(name)+"_"+cc.Suffix)
				}
				fmt.Printf("    Env: %s%s%s\n", colorDim, strings.Join(keys, ", "), colorReset)
			}
		}
		fmt.Println()
	}

	// Dev interfaces
	if astroSpec.Dev != nil && len(astroSpec.Dev.Interfaces) > 0 {
		fmt.Printf("%s%sDev Interfaces%s  %sChannels through which users and systems reach the agent (dev overrides)%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
		for _, name := range astroSpec.Dev.Interfaces {
			fmt.Printf("  %s%s%s %senabled%s\n", colorCyan, name, colorReset, colorGreen, colorReset)
			if envs := getInterfaceEnvVars(name); len(envs) > 0 {
				fmt.Printf("    Env: %s%s%s\n", colorDim, strings.Join(envs, ", "), colorReset)
			}
		}
		fmt.Println()
	}

	// Ingestion
	if len(astroSpec.Ingestion) > 0 {
		fmt.Printf("%s%sIngestion%s  %sBackground jobs that sync data into knowledge stores%s\n", colorBold, colorBlue, colorReset, colorDim, colorReset)
		for name, ing := range astroSpec.Ingestion {
			fmt.Printf("\n  %s%s%s\n", colorCyan, name, colorReset)
			fmt.Printf("    Trigger: %s%s%s", colorYellow, ing.Trigger.Type, colorReset)
			if astroSpec.Dev != nil {
				if sched := astroSpec.Dev.Schedules[name]; sched != "" {
					fmt.Printf(" (%s)", sched)
				}
			}
			fmt.Println()
			if ing.Container.Build != nil {
				resolved := resolvePath(ing.Container.Build.Context, specDir, workingDir)
				fmt.Printf("    Build context: %s%s%s\n", colorYellow, resolved, colorReset)
			} else if ing.Container.Image != "" {
				fmt.Printf("    Image: %s%s%s\n", colorDim, ing.Container.Image, colorReset)
			}
		}
		fmt.Println()
	}

	// Warnings
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

// collectWarnings checks the spec for potential issues like overlapping env vars,
// duplicate ports, duplicate providers, and user env vars that shadow auto-injected ones.
func collectWarnings(s *spec.AstroSpec) []string {
	var warnings []string

	// Build map of auto-injected env vars -> source component for overlap detection.
	// Mirrors the logic in compose/builder.go buildEnvironment().
	autoEnv := make(map[string]string) // env key -> "source description"

	// Models inject {NAME}_HOST
	for name := range s.Models {
		key := strings.ToUpper(name) + "_HOST"
		if prev, ok := autoEnv[key]; ok {
			warnings = append(warnings, fmt.Sprintf("Env var %s%s%s is set by both %s and model %s%s%s.",
				colorBold, key, colorReset, prev, colorCyan, name, colorReset))
		}
		autoEnv[key] = fmt.Sprintf("model %s%s%s", colorCyan, name, colorReset)
	}

	// Knowledge stores inject {PROVIDER_PREFIX}_HOST and {PROVIDER_PREFIX}_PORT
	providerUsers := make(map[string][]string) // provider -> list of knowledge names
	for name, k := range s.Knowledge {
		if k.Provider == "" {
			continue
		}
		providerUsers[strings.ToLower(k.Provider)] = append(providerUsers[strings.ToLower(k.Provider)], name)

		prov := spec.GetProvider(k.Provider)
		if prov.EnvPrefix == "" {
			continue
		}
		for _, suffix := range []string{"_HOST", "_PORT"} {
			key := prov.EnvPrefix + suffix
			if prev, ok := autoEnv[key]; ok {
				warnings = append(warnings, fmt.Sprintf("Env var %s%s%s is set by both %s and knowledge %s%s%s (provider %s). Only one will take effect.",
					colorBold, key, colorReset, prev, colorCyan, name, colorReset, k.Provider))
			}
			autoEnv[key] = fmt.Sprintf("knowledge %s%s%s (provider %s)", colorCyan, name, colorReset, k.Provider)
		}
	}

	// Warn if same provider used by multiple knowledge stores
	for provider, names := range providerUsers {
		if len(names) > 1 {
			warnings = append(warnings, fmt.Sprintf("Multiple knowledge stores use provider %s%s%s: %s. They will share the same env vars and only one set of connection details will be injected.",
				colorYellow, provider, colorReset, strings.Join(names, ", ")))
		}
	}

	// Cloud provider credentials inject {NAME}_{SUFFIX}
	// Scan models for cloud providers
	for name, model := range s.Models {
		if model.IsProviderMode() {
			if suffixes := getIntegrationSuffixes(model.Provider); len(suffixes) > 0 {
				for _, suffix := range suffixes {
					key := strings.ToUpper(name) + "_" + suffix
					if prev, ok := autoEnv[key]; ok {
						warnings = append(warnings, fmt.Sprintf("Env var %s%s%s is set by both %s and model %s%s%s.",
							colorBold, key, colorReset, prev, colorCyan, name, colorReset))
					}
					autoEnv[key] = fmt.Sprintf("model %s%s%s", colorCyan, name, colorReset)
				}
			}
		}
	}
	// Scan knowledge for cloud providers
	for name, knowledge := range s.Knowledge {
		if knowledge.IsProviderMode() {
			if suffixes := getIntegrationSuffixes(knowledge.Provider); len(suffixes) > 0 {
				for _, suffix := range suffixes {
					key := strings.ToUpper(name) + "_" + suffix
					if prev, ok := autoEnv[key]; ok {
						warnings = append(warnings, fmt.Sprintf("Env var %s%s%s is set by both %s and knowledge %s%s%s.",
							colorBold, key, colorReset, prev, colorCyan, name, colorReset))
					}
					autoEnv[key] = fmt.Sprintf("knowledge %s%s%s", colorCyan, name, colorReset)
				}
			}
		}
	}
	// Scan tools for cloud providers
	for name, tool := range s.Tools {
		if tool.IsProviderMode() {
			if suffixes := getIntegrationSuffixes(tool.Provider); len(suffixes) > 0 {
				for _, suffix := range suffixes {
					key := strings.ToUpper(name) + "_" + suffix
					if prev, ok := autoEnv[key]; ok {
						warnings = append(warnings, fmt.Sprintf("Env var %s%s%s is set by both %s and tool %s%s%s.",
							colorBold, key, colorReset, prev, colorCyan, name, colorReset))
					}
					autoEnv[key] = fmt.Sprintf("tool %s%s%s", colorCyan, name, colorReset)
				}
			}
		}
	}
	// Integrations
	for name, integration := range s.Integrations {
		for _, cc := range integration.Credentials {
			key := strings.ToUpper(name) + "_" + cc.Suffix
			if prev, ok := autoEnv[key]; ok {
				warnings = append(warnings, fmt.Sprintf("Env var %s%s%s is set by both %s and integration %s%s%s.",
					colorBold, key, colorReset, prev, colorCyan, name, colorReset))
			}
			autoEnv[key] = fmt.Sprintf("integration %s%s%s", colorCyan, name, colorReset)
		}
	}

	// System-reserved env vars
	reserved := map[string]string{
		"GRPC_SERVER_ADDR":            "interfaces (messaging sidecar)",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "observability collector",
		"AGENT_URL":                   "system",
		"AGENT_HOST":                  "system",
		"ASTRO_AGENT_NAME":            "system",
		"ASTRO_AGENT_BUILD_ID":        "system",
	}
	for key, source := range reserved {
		autoEnv[key] = source
	}

	// Check user-provided container.environment for shadows against auto-injected vars.
	checkUserEnv := func(env map[string]string, component string) {
		for key := range env {
			if source, ok := autoEnv[key]; ok {
				warnings = append(warnings, fmt.Sprintf("%s sets env var %s%s%s which shadows the value auto-injected by %s.",
					component, colorBold, key, colorReset, source))
			}
		}
	}

	for name, model := range s.Models {
		mc := model.ResolvedContainer()
		checkUserEnv(mc.Environment, fmt.Sprintf("Model %s%s%s", colorCyan, name, colorReset))
	}
	for name, k := range s.Knowledge {
		if k.Container != nil {
			checkUserEnv(k.Container.Environment, fmt.Sprintf("Knowledge %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, tool := range s.Tools {
		if tool.Container != nil {
			checkUserEnv(tool.Container.Environment, fmt.Sprintf("Tool %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, ing := range s.Ingestion {
		checkUserEnv(ing.Container.Environment, fmt.Sprintf("Ingestion %s%s%s", colorCyan, name, colorReset))
	}

	// Check for duplicate ports across components
	portUsers := make(map[int][]string) // port -> list of component descriptions
	for name, model := range s.Models {
		mc := model.ResolvedContainer()
		if mc.Port > 0 {
			portUsers[mc.Port] = append(portUsers[mc.Port],
				fmt.Sprintf("model %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, k := range s.Knowledge {
		kc := k.ResolvedContainer()
		if kc.Port > 0 {
			portUsers[kc.Port] = append(portUsers[kc.Port],
				fmt.Sprintf("knowledge %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, tool := range s.Tools {
		if tool.Container != nil && tool.Container.Port > 0 {
			portUsers[tool.Container.Port] = append(portUsers[tool.Container.Port],
				fmt.Sprintf("tool %s%s%s", colorCyan, name, colorReset))
		}
	}
	for name, ing := range s.Ingestion {
		if ing.Container.Port > 0 {
			portUsers[ing.Container.Port] = append(portUsers[ing.Container.Port],
				fmt.Sprintf("ingestion %s%s%s", colorCyan, name, colorReset))
		}
	}
	for port, users := range portUsers {
		if len(users) > 1 {
			warnings = append(warnings, fmt.Sprintf("Port %s%d%s is used by multiple components: %s.",
				colorBold, port, colorReset, strings.Join(users, ", ")))
		}
	}

	return warnings
}

// getInterfaceEnvVars returns the env vars produced by a given interface.
// Mirrors the switch in compose/builder.go buildMessagingEnvironment().
func getInterfaceEnvVars(name string) []string {
	switch strings.ToLower(name) {
	case "slack":
		return []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"}
	case "web":
		return []string{"WEB_ENABLED", "WEB_LISTEN_ADDR"}
	default:
		return nil
	}
}

// getIntegrationSuffixes returns the env var suffixes for a supported provider.
// Mirrors getProviderCredentialSuffixes in compose/builder.go.
func getIntegrationSuffixes(provider string) []string {
	switch strings.ToLower(provider) {
	case "anthropic", "openai", "google", "gemini", "cohere", "pinecone":
		return []string{"API_KEY"}
	case "github", "gitlab":
		return []string{"TOKEN"}
	case "slack":
		return []string{"BOT_TOKEN", "APP_TOKEN"}
	default:
		return nil
	}
}
