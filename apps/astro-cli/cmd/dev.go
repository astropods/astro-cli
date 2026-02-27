package cmd

import (
	"bytes"
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

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	composeBuilder "github.com/postman/astro/apps/astro-cli/internal/compose"
	"github.com/postman/astro/apps/astro-cli/internal/config"
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

// checkDockerRunning verifies Docker is installed and the daemon is accessible.
func checkDockerRunning() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows is not supported — please use macOS or Linux") //nolint:staticcheck
	}

	if _, err := exec.LookPath("docker"); err != nil {
		msg := "Docker is not installed."
		if runtime.GOOS == "darwin" {
			msg += "\n  → Download Docker Desktop for Mac: https://docs.docker.com/desktop/install/mac-install/"
		} else {
			msg += "\n  → Install Docker Engine: https://docs.docker.com/engine/install/"
		}
		return fmt.Errorf("%s", msg)
	}

	cmd := exec.Command("docker", "info")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		red := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
		dim := lipgloss.NewStyle().Faint(true)
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

		var hint2 string
		if runtime.GOOS == "darwin" {
			hint2 = hint.Render("→ Open Docker Desktop from your Applications folder or system tray")
		} else {
			hint2 = hint.Render("→ Run: sudo systemctl start docker")
		}

		msg := red.Render("🐳 Docker is not running") + "\n" +
			dim.Render("Start Docker and re-run your command.") + "\n\n" +
			hint2
		return fmt.Errorf("%s", msg)
	}
	return nil
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
	if err := checkDockerRunning(); err != nil {
		return err
	}

	verbose, _ := cmd.Flags().GetBool("verbose")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPath(cmd, workingDir)
	if err != nil {
		return err
	}

	if localReset {
		if err := unlinkLocalPackages(workingDir); err != nil {
			return fmt.Errorf("local-reset: %w", err)
		}
		fmt.Printf("%s→%s Removed local packages. Run 'bun install' to restore dependencies.\n", colorCyan, colorReset)
		return nil
	}

	// --local implies --no-pull and requires ASTRO_ROOT for local packages
	if local {
		noPull = true
		if os.Getenv("ASTRO_ROOT") == "" {
			return fmt.Errorf("ASTRO_ROOT is not set (required for --local to use local packages)")
		}
	}

	printBanner()
	fmt.Printf("%s→%s Loading spec: %s\n", colorCyan, colorReset, filepath.Base(specPath))

	// Parse Astro spec
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	fmt.Printf("%s→%s Agent: %s%s%s\n", colorCyan, colorReset, colorBold, astroSpec.Name, colorReset)

	// Load .env file
	envVars, err := utils.LoadEnvFile(workingDir, envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}
	if envVars == nil {
		envVars = make(map[string]string)
	} else {
		fmt.Printf("%s→%s Environment: %d variable(s) from %s\n", colorCyan, colorReset, len(envVars), envFile)
		for key, val := range envVars {
			if err := os.Setenv(key, val); err != nil {
				return fmt.Errorf("failed to set env var %s: %w", key, err)
			}
		}
	}

	// Load stored project config and merge (config store takes priority over .env)
	storedVars := config.GetProjectVars(binaryName, workingDir)
	for k, v := range storedVars {
		envVars[k] = v
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("failed to set env var %s: %w", k, err)
		}
	}
	if len(storedVars) > 0 {
		fmt.Printf("%s→%s Config: %d variable(s) from project store\n", colorCyan, colorReset, len(storedVars))
	} else if len(envVars) == 0 {
		fmt.Printf("%s→%s %sNo credentials found. Run 'ast configure' to set up.%s\n", colorCyan, colorReset, colorDim, colorReset)
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
	fmt.Printf("%s→%s Services: %s\n", colorCyan, colorReset, strings.Join(serviceNames, ", "))

	// Build all services upfront — including profiled ingestion containers — so
	// startup ingestions don't get built lazily after everything else is running.
	buildArgs := []string{"compose", "--profile", "ingestion", "-f", cPath, "build"}
	if rebuild {
		buildArgs = append(buildArgs, "--no-cache")
	}
	if noPull {
		buildArgs = append(buildArgs, "--pull=never")
	}
	buildTitle := "Building services..."
	if rebuild {
		buildTitle = "Building services (no cache)..."
	}
	if err := withSpinner(buildTitle, verbose, func() error {
		return runCmd(exec.Command("docker", buildArgs...), verbose) //nolint:gosec
	}); err != nil {
		return fmt.Errorf("failed to build services: %w", err)
	}

	// Start non-profiled services (already built above)
	if err := withSpinner("Starting services...", verbose, func() error {
		return runCmd(exec.Command("docker", "compose", "-f", cPath, "up", "-d", "--no-build"), verbose) //nolint:gosec
	}); err != nil {
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
	runStartupIngestions(astroSpec, cPath, verbose)

	printReadyBlock(astroSpec, hasWebInterface)
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
	fmt.Printf("%s→%s Using local packages from %s\n", colorCyan, colorReset, astroRoot)

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
	fmt.Printf("%s→%s Agent running as local process %s(%s)%s\n", colorCyan, colorReset, colorDim, startCommand, colorReset)

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
	fmt.Printf("%s→%s %sReady! Press Ctrl+C to stop%s\n", colorCyan, colorReset, colorBold, colorReset)
	fmt.Println()

	<-sigChan

	fmt.Println()
	fmt.Printf("%s→%s Shutting down...\n", colorCyan, colorReset)

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

	fmt.Printf("%s→%s Cleanup complete\n", colorCyan, colorReset)
	fmt.Printf("  %sTip: run 'ast dev --local-reset' to remove injected local dependencies%s\n", colorDim, colorReset)

	return nil
}

func runDevLogs(cmd *cobra.Command, args []string) error {
	if err := checkDockerRunning(); err != nil {
		return err
	}

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
	if err := checkDockerRunning(); err != nil {
		return err
	}

	cPath, err := composePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cPath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment found (missing %s). Run 'ast dev' first", cPath)
	}

	fmt.Printf("%s→%s Stopping dev containers...\n", colorCyan, colorReset)
	downCmd := exec.Command("docker", "compose", "-f", cPath, "down") //nolint:gosec
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	if err := downCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	fmt.Printf("%s→%s Containers stopped\n", colorCyan, colorReset)
	return nil
}

func runDevTrigger(cmd *cobra.Command, args []string) error {
	if err := checkDockerRunning(); err != nil {
		return err
	}

	specPath, err := resolveSpecPathFromCwd(cmd)
	if err != nil {
		return err
	}

	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	// No name given — list available ingestion jobs and exit
	if len(args) == 0 {
		if len(astroSpec.Ingestion) == 0 {
			return fmt.Errorf("no ingestion jobs defined in %s", filepath.Base(specPath))
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
		return fmt.Errorf("ingestion job %q not found in %s", name, filepath.Base(specPath))
	}

	cPath, err := composePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(cPath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment found (missing %s). Run 'ast dev' first", cPath)
	}

	fmt.Printf("%s→%s Triggering ingestion: %s\n", colorCyan, colorReset, name)
	runCmd := exec.Command("docker", "compose", "-f", cPath, "run", "--rm", fmt.Sprintf("ingestion-%s", name)) //nolint:gosec
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		return fmt.Errorf("ingestion '%s' failed: %w", name, err)
	}
	fmt.Printf("%s→%s Ingestion '%s' completed\n", colorCyan, colorReset, name)
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
func runStartupIngestions(s *spec.AstroSpec, cPath string, verbose bool) {
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "startup" {
			continue
		}
		if err := withSpinner(fmt.Sprintf("Running ingestion: %s...", name), verbose, func() error {
			return runCmd(exec.Command("docker", "compose", "-f", cPath, "run", "--rm", fmt.Sprintf("ingestion-%s", name)), verbose) //nolint:gosec
		}); err != nil {
			fmt.Fprintf(os.Stderr, "%s✗%s Startup ingestion '%s' failed: %v\n", colorRed, colorReset, name, err)
		}
	}
}

// verboseWriter returns stdout when verbose is true, io.Discard otherwise.
func verboseWriter(verbose bool) io.Writer {
	if verbose {
		return os.Stdout
	}
	return io.Discard
}

// runCmd runs cmd with stdout routed through verboseWriter.
// In verbose mode stderr goes directly to os.Stderr.
// In non-verbose mode stderr is captured; on failure it is flushed to os.Stderr
// so docker's error output is never silently discarded.
func runCmd(cmd *exec.Cmd, verbose bool) error {
	cmd.Stdout = verboseWriter(verbose)
	if verbose {
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if errBuf.Len() > 0 {
			fmt.Fprint(os.Stderr, errBuf.String())
		}
		return err
	}
	return nil
}

// withSpinner runs fn with an animated spinner title in non-verbose mode, or
// prints the title and runs fn directly in verbose mode.
func withSpinner(title string, verbose bool, fn func() error) error {
	if verbose {
		fmt.Println(title)
		return fn()
	}
	return spinner.New().
		Title(" " + title).
		Style(lipgloss.NewStyle().Foreground(lipgloss.Color("6"))).
		ActionWithErr(func(_ context.Context) error {
			return fn()
		}).
		Run()
}

// printReadyBlock renders the post-start summary using lipgloss.
func printReadyBlock(s *spec.AstroSpec, hasWebInterface bool) {
	teal := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	bold := lipgloss.NewStyle().Bold(true)
	boldTeal := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dim := lipgloss.NewStyle().Faint(true)

	var lines []string
	lines = append(lines, "✨ "+bold.Render(s.Name)+" is ready")
	lines = append(lines, "")

	if hasWebInterface {
		lines = append(lines, teal.Render("➜")+"  "+boldTeal.Render("http://localhost:3000"))
		lines = append(lines, dim.Render("   http://localhost:3100  (API)"))
	}

	// Webhook endpoints
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "webhook" {
			continue
		}
		port := ingestion.Container.Port
		if port == 0 {
			port = 3001
		}
		lines = append(lines, dim.Render(fmt.Sprintf("   http://localhost:%d  (%s webhook)", port, name)))
	}

	lines = append(lines, "")
	lines = append(lines, bold.Render("ast dev logs")+"  — tail logs")
	lines = append(lines, bold.Render("ast dev stop")+"  — stop")

	// Manual / schedule ingestion hints
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "schedule" && ingestion.Trigger.Type != "manual" {
			continue
		}
		lines = append(lines, bold.Render(fmt.Sprintf("ast dev trigger %-8s", name))+"— trigger ingestion")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Println()
	fmt.Println(box)
	fmt.Println()
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
		fmt.Printf("%s!%s %sUnable to open browser automatically on %s%s\n", colorYellow, colorReset, colorDim, runtime.GOOS, colorReset)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Printf("%s✗%s %sFailed to open browser: %v%s\n", colorRed, colorReset, colorDim, err, colorReset)
	}
}
