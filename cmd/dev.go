package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	composeBuilder "github.com/postman/astro/apps/astro-cli/internal/compose"
	"github.com/postman/astro/apps/astro-cli/internal/utils"
	spec "github.com/postman/astro/packages/astro-spec"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Manage local development environment",
	Long: `Manage the local development environment for your agent.

Subcommands:
  start   Start dev containers (default when no subcommand given)
  logs    Tail container logs
  stop    Stop dev containers

Running 'ast dev' without a subcommand is equivalent to 'ast dev start'.

Example:
  ast dev                  # start containers and exit
  ast dev start --rebuild  # force rebuild containers
  ast dev logs             # tail logs
  ast dev stop             # stop containers
  ast dev --local          # run agent as local process (blocking)`,
	RunE: runDevStart,
}

var devStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start dev containers",
	Long:  `Start the local development environment with Docker containers. In non-local mode, containers start in background and the command exits. Use 'ast dev logs' to tail logs and 'ast dev stop' to stop.`,
	RunE:  runDevStart,
}

var devLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Tail container logs",
	Long:  `Tail logs from the running dev containers. Defaults to the agent container. Optionally specify a service name (e.g. astro-messaging, playground) to tail a different container.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDevLogs,
}

var devStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop dev containers",
	Long:  `Stop and remove the running dev containers.`,
	RunE:  runDevStop,
}

var devTriggerCmd = &cobra.Command{
	Use:   "trigger <name>",
	Short: "Trigger an ingestion job",
	Long:  `Manually trigger a named ingestion job. Runs the ingestion container and exits when done.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDevTrigger,
}

var (
	envFile    string
	rebuild    bool
	noPull     bool
	local      bool
	localReset bool
)

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devStartCmd)
	devCmd.AddCommand(devLogsCmd)
	devCmd.AddCommand(devStopCmd)
	devCmd.AddCommand(devTriggerCmd)

	// Flags on both devCmd and devStartCmd so they work with `ast dev` and `ast dev start`
	for _, cmd := range []*cobra.Command{devCmd, devStartCmd} {
		cmd.Flags().StringVar(&envFile, "env", utils.DefaultEnvFile, "Environment file for integration credentials")
		cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force rebuild all containers without cache")
		cmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip pulling images (use only locally built images)")
		cmd.Flags().BoolVar(&local, "local", false, "Use local images, no pull, run agent as local process (bun); implies --no-pull")
		cmd.Flags().BoolVar(&localReset, "local-reset", false, "Remove local package (use after ast dev --local); run 'bun install' to restore deps")
		_ = cmd.Flags().MarkHidden("local")
		_ = cmd.Flags().MarkHidden("local-reset")
	}
}

// composePath returns the path to the docker-compose.yml for the current working directory.
func composePath() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(workingDir, ".ast", "docker-compose.yml"), nil
}

func runDevStart(cmd *cobra.Command, args []string) error {
	// Get spec file path
	specFile, err := specFilePath(cmd)
	if err != nil {
		return err
	}
	verbose, _ := cmd.Flags().GetBool("verbose")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	if localReset {
		if err := unlinkLocalPackages(workingDir); err != nil {
			return fmt.Errorf("local-reset: %w", err)
		}
		fmt.Println("📦 Removed local packages. Run 'bun install' to restore dependencies.")
		return nil
	}

	// --local implies --no-pull and requires ASTRO_ROOT for local packages
	if local {
		noPull = true
		if os.Getenv("ASTRO_ROOT") == "" {
			return fmt.Errorf("ASTRO_ROOT is not set (required for --local to use local packages)")
		}
	}

	fmt.Printf("🚀 Starting Astro dev mode...\n")
	fmt.Printf("📄 Loading spec from: %s\n", specFile)

	// Parse astroai.yml
	specPath := filepath.Join(workingDir, specFile)
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	fmt.Printf("✅ Loaded spec for agent: %s\n", astroSpec.Name)

	// Load .env file
	envVars, err := utils.LoadEnvFile(workingDir, envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}
	if envVars == nil {
		envVars = make(map[string]string)
		fmt.Printf("⚠️  No .env file found at %s (continuing without integration credentials)\n", envFile)
	} else {
		fmt.Printf("🔑 Loading environment from: %s\n", envFile)
		var envKeys []string
		for key := range envVars {
			envKeys = append(envKeys, key)
		}
		fmt.Printf("   Loaded %d environment variables: %s\n", len(envKeys), strings.Join(envKeys, ", "))
		for key, val := range envVars {
			if err := os.Setenv(key, val); err != nil {
				return fmt.Errorf("failed to set env var %s: %w", key, err)
			}
		}
	}
	// Build Docker Compose project
	project, err := composeBuilder.BuildProject(astroSpec, workingDir, envVars)
	if err != nil {
		return fmt.Errorf("failed to build compose project: %w", err)
	}

	// Strip remote registry prefix from images so we use locally built images
	if local {
		for name, svc := range project.Services {
			if svc.Image != "" {
				svc.Image = utils.ImageNameForLocal(svc.Image, true)
				project.Services[name] = svc
			}
		}
		if verbose {
			fmt.Println("   --local: using local image names (no pull)")
		}
	}

	// --local: omit agent from compose and run it as a local process
	if local {
		delete(project.Services, "agent")
		if verbose {
			fmt.Println("   --local: agent will run as local process")
		}
	}

	// Write docker-compose.yml file
	cPath := filepath.Join(workingDir, ".ast", "docker-compose.yml")
	if err := os.MkdirAll(filepath.Dir(cPath), 0755); err != nil { //nolint:gosec
		return fmt.Errorf("failed to create .ast directory: %w", err)
	}

	composeData, err := yaml.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal compose project: %w", err)
	}

	if err := os.WriteFile(cPath, composeData, 0644); err != nil { //nolint:gosec
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	if verbose {
		fmt.Printf("   Wrote compose file to: %s\n", cPath)
	}

	// Log services before building
	serviceNames := make([]string, 0, len(project.Services))
	for name := range project.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	fmt.Printf("📦 %d service(s): %s\n", len(serviceNames), strings.Join(serviceNames, ", "))

	// Build all services upfront — including profiled ingestion containers — so
	// startup ingestions don't get built lazily after everything else is running.
	fmt.Println("🔨 Building services...")
	buildArgs := []string{"compose", "--profile", "ingestion", "-f", cPath, "build"}
	if rebuild {
		fmt.Println("   Using --no-cache for clean rebuild...")
		buildArgs = append(buildArgs, "--no-cache")
	}
	if noPull {
		buildArgs = append(buildArgs, "--pull=never")
	}
	buildCmd := exec.Command("docker", buildArgs...) //nolint:gosec
	buildCmd.Stdout = verboseWriter(verbose)
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build services: %w", err)
	}

	// Start non-profiled services (already built above)
	fmt.Println("🚀 Starting services...")
	upArgs := []string{"compose", "-f", cPath, "up", "-d", "--no-build"}
	upCmd := exec.Command("docker", upArgs...) //nolint:gosec
	upCmd.Stdout = verboseWriter(verbose)
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	fmt.Println()
	// Check if messaging interface is configured (from dev section)
	hasWebInterface := false
	if astroSpec.Dev != nil {
		for _, name := range astroSpec.Dev.Interfaces {
			if name == "web" {
				hasWebInterface = true
			}
		}
	}

	// --local: run agent as local process and block
	if local {
		return runLocalAgent(cmd, astroSpec, workingDir, cPath, envVars, hasWebInterface)
	}

	// Run startup ingestions before printing the ready block so output isn't interleaved
	runStartupIngestions(astroSpec, cPath)

	// Print unified ready block
	fmt.Println()
	fmt.Println("✅ All services running!")
	fmt.Println()
	if hasWebInterface {
		fmt.Println("  Your agent is ready. Open the playground to start chatting:")
		fmt.Println()
		fmt.Printf("  %s%s➜  http://localhost:3000%s\n", colorBold, colorGreen, colorReset)
		fmt.Println()
		fmt.Printf("  %sAPI  http://localhost:3100%s\n", colorDim, colorReset)
	}
	for name, ingestion := range astroSpec.Ingestion {
		if ingestion.Trigger.Type != "webhook" {
			continue
		}
		port := ingestion.Container.Port
		if port == 0 {
			port = 3001
		}
		fmt.Printf("  %sWebhook  http://localhost:%d%s  (%s)\n", colorDim, port, colorReset, name)
	}
	fmt.Println()
	fmt.Printf("  %sast dev logs%s         — tail logs\n", colorBold, colorReset)
	fmt.Printf("  %sast dev stop%s         — stop\n", colorBold, colorReset)
	printIngestionHints(astroSpec)
	fmt.Println()

	return nil
}

// runLocalAgent runs the agent as a local bun process and blocks until Ctrl+C.
func runLocalAgent(_ *cobra.Command, astroSpec *spec.AstroSpec, workingDir, cPath string, envVars map[string]string, hasWebInterface bool) error {
	agentCtx, agentCancel := context.WithCancel(context.Background())
	agentEnv := buildLocalAgentEnv(astroSpec, envVars)

	// Use local @saswatds/* packages from ASTRO_ROOT
	astroRoot, err := resolveAstroSourceRoot()
	if err != nil {
		agentCancel()
		return err
	}
	if err := linkLocalPackages(workingDir, astroRoot); err != nil {
		agentCancel()
		return fmt.Errorf("link local packages: %w", err)
	}
	// Ensure messaging SDK is built (package main points to dist/index.js)
	msgSDK := filepath.Join(astroRoot, "packages", "messaging", "sdk", "node")
	if _, err := os.Stat(filepath.Join(msgSDK, "dist", "index.js")); err != nil {
		agentCancel()
		return fmt.Errorf("messaging SDK not built: run 'moon run messaging:sdk-build' first")
	}
	// Ensure SDKs are built
	adaptersRoot := filepath.Join(astroRoot, "packages", "adapters")
	adapterCore := filepath.Join(adaptersRoot, "packages", "core")
	if _, err := os.Stat(filepath.Join(adapterCore, "dist", "index.js")); err != nil {
		agentCancel()
		return fmt.Errorf("adapters not built: run 'moon run adapters:build' first")
	}
	fmt.Printf("📦 Using local packages from %s\n", astroRoot)

	// Resolve start command from spec (default: "bun --watch run start")
	startCommand := "bun --watch run start"
	if astroSpec.Dev != nil && astroSpec.Dev.Command != "" {
		startCommand = astroSpec.Dev.Command
	}

	// Run via shell so the command string is interpreted correctly
	agentCmd := exec.CommandContext(agentCtx, "sh", "-c", startCommand) //nolint:gosec
	agentCmd.Dir = workingDir
	agentCmd.Env = agentEnv
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	if err := agentCmd.Start(); err != nil {
		agentCancel()
		return fmt.Errorf("failed to start agent: %w", err)
	}
	fmt.Printf("🤖 Agent running as local process (%s)\n", startCommand)

	if hasWebInterface {
		// Open playground in browser
		go func() {
			time.Sleep(2 * time.Second)
			openBrowser("http://localhost:3000")
		}()
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	fmt.Println()
	fmt.Println("✨ Ready! Press Ctrl+C to stop")
	fmt.Println()

	<-sigChan

	fmt.Println()
	fmt.Println("🛑 Shutting down...")

	agentCancel()
	if agentCmd.Process != nil {
		_ = agentCmd.Process.Kill()
	}

	// Stop all services
	downCmd := exec.Command("docker", "compose", "-f", cPath, "down") //nolint:gosec
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	if err := downCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	fmt.Println("✅ Cleanup complete")
	fmt.Println("💡 Tip: run 'ast dev --local-reset' to remove injected local dependencies")

	return nil
}

func runDevLogs(cmd *cobra.Command, args []string) error {
	cPath, err := composePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cPath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment found (missing %s). Run 'ast dev' first", cPath)
	}

	service := "agent"
	if len(args) > 0 {
		service = args[0]
	}
	logsArgs := []string{"compose", "-f", cPath, "logs", "-f", service}

	logsCmd := exec.Command("docker", logsArgs...) //nolint:gosec
	logsCmd.Stdout = os.Stdout
	logsCmd.Stderr = os.Stderr

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if logsCmd.Process != nil {
			_ = logsCmd.Process.Signal(syscall.SIGTERM)
		}
	}()

	return logsCmd.Run()
}

func runDevStop(cmd *cobra.Command, args []string) error {
	cPath, err := composePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cPath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment found (missing %s). Run 'ast dev' first", cPath)
	}

	fmt.Println("🛑 Stopping dev containers...")
	downCmd := exec.Command("docker", "compose", "-f", cPath, "down") //nolint:gosec
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	if err := downCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	fmt.Println("✅ Containers stopped")
	return nil
}

func runDevTrigger(cmd *cobra.Command, args []string) error {
	specFile, err := specFilePath(cmd)
	if err != nil {
		return err
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	astroSpec, err := spec.ParseSpec(filepath.Join(workingDir, specFile))
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	// No name given — list available ingestion jobs and exit
	if len(args) == 0 {
		if len(astroSpec.Ingestion) == 0 {
			return fmt.Errorf("no ingestion jobs defined in %s", specFile)
		}
		fmt.Println("Available ingestion jobs:")
		fmt.Println()
		for name, ing := range astroSpec.Ingestion {
			fmt.Printf("  %s%s%s  %s(%s)%s\n", colorBold, name, colorReset, colorDim, ing.Trigger.Type, colorReset)
		}
		fmt.Println()
		fmt.Printf("Run %sast dev trigger <name>%s to trigger one.\n", colorBold, colorReset)
		return nil
	}

	name := args[0]

	// Validate the name exists in the spec
	if _, ok := astroSpec.Ingestion[name]; !ok {
		fmt.Fprintf(os.Stderr, "Unknown ingestion job %q. Available:\n\n", name)
		for n := range astroSpec.Ingestion {
			fmt.Fprintf(os.Stderr, "  %s\n", n)
		}
		fmt.Fprintln(os.Stderr)
		return fmt.Errorf("ingestion job %q not found in %s", name, specFile)
	}

	cPath, err := composePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cPath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment found (missing %s). Run 'ast dev' first", cPath)
	}

	fmt.Printf("🔄 Triggering ingestion: %s\n", name)
	runCmd := exec.Command("docker", "compose", "-f", cPath, "run", "--rm", fmt.Sprintf("ingestion-%s", name)) //nolint:gosec
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		return fmt.Errorf("ingestion '%s' failed: %w", name, err)
	}
	fmt.Printf("✅ Ingestion '%s' completed\n", name)
	return nil
}

// resolveAstroSourceRoot returns the Astro monorepo root from ASTRO_ROOT.
// Used in --local to link @saswatds/* from packages/.
func resolveAstroSourceRoot() (string, error) {
	p := os.Getenv("ASTRO_ROOT")
	if p == "" {
		return "", fmt.Errorf("ASTRO_ROOT is not set (required for --local to use local packages)")
	}
	return filepath.Clean(p), nil
}

// localPackage describes a package to link in --local mode (scope, name, path relative to astroRoot).
type localPackage struct {
	scope string // e.g. "@astropods"
	name  string // e.g. "adapter-mastra"
	path  string // relative to astroRoot, e.g. "packages/adapters/mastra"
}

// localAstroPackages are the packages we link in --local and remove in --local-reset.
var localAstroPackages = []localPackage{
	{"@astropods", "messaging", "packages/messaging/sdk/node"},
	{"@astropods", "adapter-core", "packages/adapters/packages/core"},
	{"@astropods", "adapter-mastra", "packages/adapters/packages/mastra"},
}

// linkLocalPackages symlinks node_modules/<scope>/<name> to the given Astro repo path
// so the agent uses local source in --local mode.
func linkLocalPackages(workingDir, astroRoot string) error {
	for _, pkg := range localAstroPackages {
		scopeDir := filepath.Join(workingDir, "node_modules", pkg.scope)
		if err := os.MkdirAll(scopeDir, 0755); err != nil { //nolint:gosec
			return err
		}
		target := filepath.Join(astroRoot, pkg.path)
		target, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if st, err := os.Stat(target); err != nil {
			return fmt.Errorf("%s/%s: %w", pkg.scope, pkg.name, err)
		} else if !st.IsDir() {
			return fmt.Errorf("%s/%s is not a directory", pkg.scope, pkg.name)
		}
		link := filepath.Join(scopeDir, pkg.name)
		_ = os.RemoveAll(link)
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s/%s: %w", pkg.scope, pkg.name, err)
		}
	}
	return nil
}

// unlinkLocalPackages removes the symlinks created by linkLocalPackages
// so the user can run bun install to restore registry dependencies.
func unlinkLocalPackages(workingDir string) error {
	for _, pkg := range localAstroPackages {
		path := filepath.Join(workingDir, "node_modules", pkg.scope, pkg.name)
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s/%s: %w", pkg.scope, pkg.name, err)
		}
	}
	return nil
}

// buildLocalAgentEnv returns env for the agent process when running with --no-container.
// Uses .env vars and sets GRPC_SERVER_ADDR=localhost:9090 so the agent talks to the messaging container.
func buildLocalAgentEnv(s *spec.AstroSpec, envVars map[string]string) []string {
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if i := strings.Index(e, "="); i > 0 {
			envMap[e[:i]] = e[i+1:]
		}
	}
	for k, v := range envVars {
		envMap[k] = v
	}
	if s.Dev != nil && len(s.Dev.Interfaces) > 0 {
		envMap["GRPC_SERVER_ADDR"] = "localhost:9090"
	}
	// Point at the collector container's published port for auto OTel
	envMap["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://localhost:4318"
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	return out
}

// runStartupIngestions runs each startup-type ingestion synchronously before the CLI exits.
func runStartupIngestions(s *spec.AstroSpec, cPath string) {
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "startup" {
			continue
		}
		fmt.Printf("🚀 Running startup ingestion: %s\n", name)
		runCmd := exec.Command("docker", "compose", "-f", cPath, "run", "--rm", fmt.Sprintf("ingestion-%s", name)) //nolint:gosec
		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr
		if err := runCmd.Run(); err != nil {
			fmt.Printf("❌ Failed to run startup ingestion '%s': %v\n", name, err)
		} else {
			fmt.Printf("✅ Startup ingestion '%s' completed\n", name)
		}
	}
}

// printIngestionHints prints manual trigger instructions for schedule and manual ingestions.
func printIngestionHints(s *spec.AstroSpec) {
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "schedule" && ingestion.Trigger.Type != "manual" {
			continue
		}
		fmt.Printf("  %sast dev trigger %-8s%s — trigger ingestion\n", colorBold, name, colorReset)
	}
}

// verboseWriter returns stdout when verbose is true, io.Discard otherwise.
func verboseWriter(verbose bool) io.Writer {
	if verbose {
		return os.Stdout
	}
	return io.Discard
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:gosec
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec
	default:
		fmt.Printf("⚠️  Unable to open browser automatically on %s\n", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("⚠️  Failed to open browser: %v\n", err)
	}
}
