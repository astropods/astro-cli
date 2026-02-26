package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/spf13/cobra"

	composeBuilder "github.com/astropods/astro/apps/astro-cli/internal/compose"
	"github.com/astropods/astro/apps/astro-cli/internal/config"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	"github.com/astropods/astro/apps/astro-cli/internal/utils"
	spec "github.com/astropods/astro/packages/astro-spec"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Manage local development environment",
	RunE:  runDevStart,
}

var devStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start dev containers",
	RunE:  runDevStart,
}

var devLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Tail container logs",
	Long:  `Tail logs from the running dev containers. Defaults to the agent container. Use --all to tail all services. Optionally specify a service name (e.g. astro-messaging, playground) to tail a different container.`,
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
	logsAll    bool
)

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devStartCmd)
	devCmd.AddCommand(devLogsCmd)
	devCmd.AddCommand(devStopCmd)
	devCmd.AddCommand(devTriggerCmd)

	devCmd.Long = fmt.Sprintf(`Manage the local development environment for your agent.

Subcommands:
  start   Start dev containers (default when no subcommand given)
  logs    Tail container logs
  stop    Stop dev containers

Running '%[1]s dev' without a subcommand is equivalent to '%[1]s dev start'.`, binaryName)

	devCmd.Example = fmt.Sprintf(`  %[1]s dev                  # start containers and exit
  %[1]s dev start --rebuild  # force rebuild containers
  %[1]s dev logs             # tail agent logs
  %[1]s dev logs --all       # tail all service logs
  %[1]s dev stop             # stop containers
  %[1]s dev --local          # run agent as local process (blocking)`, binaryName)

	devStartCmd.Long = fmt.Sprintf(`Start the local development environment with Docker containers. In non-local mode, containers start in background and the command exits. Use '%[1]s dev logs' to tail logs and '%[1]s dev stop' to stop.`, binaryName)

	// Flags on both devCmd and devStartCmd
	for _, cmd := range []*cobra.Command{devCmd, devStartCmd} {
		cmd.Flags().StringVar(&envFile, "env", utils.DefaultEnvFile, "Environment file for integration credentials")
		cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force rebuild all containers without cache")
		cmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip pulling images (use only locally built images)")
		cmd.Flags().BoolVar(&local, "local", false, "Use local images, no pull, run agent as local process (bun); implies --no-pull")
		cmd.Flags().BoolVar(&localReset, "local-reset", false, fmt.Sprintf("Remove local package (use after %s dev --local); run 'bun install' to restore deps", binaryName))
		_ = cmd.Flags().MarkHidden("local")
		_ = cmd.Flags().MarkHidden("local-reset")
	}

	devLogsCmd.Flags().BoolVar(&logsAll, "all", false, "Tail logs from all services (not just agent)")
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

// devStatePath returns the path to the dev-environment marker file.
func devStatePath() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(workingDir, ".ast", ".running"), nil
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
			return fmt.Errorf("ASTRO_ROOT is not set (required for --local to use local packages)\n\n  Set it to the path of your astro monorepo, e.g.:\n    export ASTRO_ROOT=$HOME/astro/astro")
		}
	}

	printBanner()
	fmt.Printf("%s→%s Loading spec: %s\n", colorCyan, colorReset, filepath.Base(specPath))

	// Parse Astro spec
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}
	warnDeprecatedMetaFields(specPath, workingDir)

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
		fmt.Printf("%s→%s %sNo credentials found. Run '%s configure' to set up.%s\n", colorCyan, colorReset, colorDim, binaryName, colorReset)
	}
	// Check for native Ollama — require host-installed Ollama for dev mode
	buildOpts := composeBuilder.BuildOptions{}
	if hasOllamaModels(astroSpec) {
		if err := checkNativeOllama(); err != nil {
			return err
		}
		buildOpts.NativeOllama = true
		fmt.Printf("%s→%s Using native Ollama (host-installed)\n", colorCyan, colorReset)
		if err := ensureOllamaModels(astroSpec, verbose); err != nil {
			return fmt.Errorf("failed to prepare Ollama models: %w", err)
		}
	}

	// Build Docker Compose project
	project, err := composeBuilder.BuildProject(astroSpec, workingDir, envVars, buildOpts)
	if err != nil {
		return fmt.Errorf("failed to build compose project: %w", err)
	}

	// Strip astropods/ prefix from images so we use locally built images
	if local {
		for name, svc := range project.Services {
			if svc.Image != "" && strings.HasPrefix(svc.Image, "astropods/") {
				svc.Image = utils.ImageNameForLocal(svc.Image, true)
				svc.PullPolicy = ""
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

	astDir := filepath.Join(workingDir, ".ast")
	if err := os.MkdirAll(astDir, 0755); err != nil { //nolint:gosec
		return fmt.Errorf("failed to create .ast directory: %w", err)
	}

	// Log services before building
	serviceNames := make([]string, 0, len(project.Services))
	for name := range project.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	fmt.Printf("%s→%s Services: %s\n", colorCyan, colorReset, strings.Join(serviceNames, ", "))

	// --local: build Docker images for services that don't have a compose build directive.
	// These are pre-built images (messaging, playground) that compose can't build on its own.
	if local {
		if err := buildLocalImages(devLocalImages, rebuild); err != nil {
			return err
		}
	}

	svc, err := newComposeService(verbose)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}

	// Build all services upfront — including profiled ingestion containers — so
	// startup ingestions don't get built lazily after everything else is running.
	buildTitle := "Building services..."
	if rebuild {
		buildTitle = "Building services (no cache)..."
	}
	if err := withSpinner(buildTitle, "Build complete", verbose, func() error {
		return svc.Build(context.Background(), project, api.BuildOptions{
			NoCache:  rebuild,
			Quiet:    !verbose,
			Services: allServiceNames(project),
		})
	}); err != nil {
		return fmt.Errorf("failed to build services: %w", err)
	}

	// Tear down leftover containers from a previous run (e.g. force-killed with Ctrl+C).
	// This is fast and idempotent when nothing is running.
	_ = svc.Down(context.Background(), astroSpec.Name, api.DownOptions{RemoveOrphans: true})

	// Start non-profiled services (already built above)
	upProject := projectForUp(project)
	if err := withSpinner("Starting services...", "Services started", verbose, func() error {
		return svc.Up(context.Background(), upProject, api.UpOptions{
			Create: api.CreateOptions{
				Build:         nil,   // --no-build (already built above)
				RemoveOrphans: true,  // remove stale containers from previous runs
			},
			Start: api.StartOptions{Project: upProject}, // pass project to skip container label lookup
		})
	}); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	// Write marker so subcommands (logs, stop, trigger) know dev is running
	if err := os.WriteFile(filepath.Join(astDir, ".running"), nil, 0644); err != nil { //nolint:gosec
		return fmt.Errorf("failed to write dev state: %w", err)
	}

	fmt.Println()
	// Check if messaging interface is configured (from dev section)
	hasWebInterface := false
	for _, name := range astroSpec.Dev.MessagingAdapters() {
		if name == "web" {
			hasWebInterface = true
			break
		}
	}

	// --local: run agent as local process and block
	if local {
		return runLocalAgent(cmd, astroSpec, workingDir, envVars, hasWebInterface)
	}

	// Run startup ingestions before printing the ready block so output isn't interleaved
	runStartupIngestions(astroSpec, project, verbose)

	printReadyBlock(astroSpec, hasWebInterface)
	return nil
}

// runLocalAgent runs the agent as a local bun process and blocks until Ctrl+C.
func runLocalAgent(_ *cobra.Command, astroSpec *spec.AstroSpec, workingDir string, envVars map[string]string, hasWebInterface bool) error {
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
	// Build local SDKs if dist/ is missing
	sdksToBuild := []struct{ name, dir string }{
		{"@astropods/messaging", filepath.Join(astroRoot, "modules", "messaging", "sdk", "node")},
		{"@astropods/adapter-core", filepath.Join(astroRoot, "modules", "adapters", "packages", "core")},
		{"@astropods/adapter-mastra", filepath.Join(astroRoot, "modules", "adapters", "packages", "mastra")},
	}
	for _, sdk := range sdksToBuild {
		if _, err := os.Stat(filepath.Join(sdk.dir, "dist", "index.js")); err != nil {
			fmt.Printf("%s→%s Building %s...\n", colorCyan, colorReset, sdk.name)
			installCmd := exec.Command("bun", "install")
			installCmd.Dir = sdk.dir
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				agentCancel()
				return fmt.Errorf("failed to install deps for %s: %w", sdk.name, err)
			}
			buildCmd := exec.Command("bun", "run", "build")
			buildCmd.Dir = sdk.dir
			buildCmd.Stdout = os.Stdout
			buildCmd.Stderr = os.Stderr
			if err := buildCmd.Run(); err != nil {
				agentCancel()
				return fmt.Errorf("failed to build %s: %w", sdk.name, err)
			}
		}
	}
	fmt.Printf("%s→%s Using local packages from %s\n", colorCyan, colorReset, astroRoot)

	// Resolve start command from spec (default: "bun --watch run start")
	startCommand := "bun --watch run start"
	if astroSpec.Dev != nil && astroSpec.Dev.Command != "" {
		startCommand = astroSpec.Dev.Command
	}

	// Run via shell so the command string is interpreted correctly.
	// Setpgid gives the process its own group so we can kill the entire tree.
	agentCmd := exec.CommandContext(agentCtx, "sh", "-c", startCommand) //nolint:gosec
	agentCmd.Dir = workingDir
	agentCmd.Env = agentEnv
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	agentCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := agentCmd.Start(); err != nil {
		agentCancel()
		return fmt.Errorf("failed to start agent: %w", err)
	}
	fmt.Printf("%s→%s Agent running as local process %s(%s)%s\n", colorCyan, colorReset, colorDim, startCommand, colorReset)

	checkComposeHealth(astroSpec.Name)

	// Stream docker compose logs in the background so service failures are visible
	logsCtx, logsCancel := context.WithCancel(context.Background())
	logsSvc, err := newComposeService(false)
	if err != nil {
		agentCancel()
		return fmt.Errorf("failed to init compose service for logs: %w", err)
	}
	go func() { //nolint:errcheck
		_ = logsSvc.Logs(logsCtx, astroSpec.Name, &stdoutLogConsumer{out: os.Stdout, err: os.Stderr},
			api.LogOptions{Follow: true})
	}()

	if hasWebInterface {
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
	signal.Stop(sigChan)

	fmt.Println()
	fmt.Printf("%s→%s Shutting down (Ctrl+C again to force)...\n", colorCyan, colorReset)

	logsCancel()
	agentCancel()
	killProcessGroup(agentCmd)

	// Stop all services
	localSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}
	if err := localSvc.Down(context.Background(), astroSpec.Name, api.DownOptions{}); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	fmt.Printf("%s→%s Cleanup complete\n", colorCyan, colorReset)
	fmt.Printf("  %sTip: run '%s dev --local-reset' to remove injected local dependencies%s\n", colorDim, binaryName, colorReset)

	return nil
}

func runDevLogs(cmd *cobra.Command, args []string) error {
	statePath, err := devStatePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment running. Run 'ast dev' first")
	}

	service := "agent"
	if len(args) > 0 {
		service = args[0]
	}

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

	logsSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}

	logsCtx, logsCancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logsCancel()
	}()

	err = logsSvc.Logs(logsCtx, astroSpec.Name, &stdoutLogConsumer{out: os.Stdout, err: os.Stderr},
		api.LogOptions{Services: []string{service}, Follow: true})
	logsCancel()
	if logsCtx.Err() != nil {
		return nil // cancelled by signal — not an error
	}
	return err
}

func runDevStop(cmd *cobra.Command, args []string) error {
	statePath, err := devStatePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment running. Run 'ast dev' first")
	}

	fmt.Println("🛑 Stopping dev containers...")
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

	stopSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}
	if err := withSpinner("Stopping services...", "Services stopped", false, func() error {
		return stopSvc.Down(context.Background(), astroSpec.Name, api.DownOptions{})
	}); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}
	_ = os.Remove(statePath)

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

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
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
		fmt.Printf("Run %s%s dev trigger <name>%s to trigger one.\n", colorBold, binaryName, colorReset)
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

	statePath, err := devStatePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment running. Run 'ast dev' first")
	}

	fmt.Printf("🔄 Triggering ingestion: %s\n", name)
	envVars, err := utils.LoadEnvFile(workingDir, envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}
	if envVars == nil {
		envVars = make(map[string]string)
	}
	ingProject, err := composeBuilder.BuildProject(astroSpec, workingDir, envVars)
	if err != nil {
		return fmt.Errorf("failed to build compose project: %w", err)
	}

	triggerSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}
	exitCode, err := triggerSvc.RunOneOffContainer(context.Background(), ingProject, api.RunOptions{
		Service:    fmt.Sprintf("ingestion-%s", name),
		AutoRemove: true,
		NoDeps:     true,
	})
	if err != nil {
		return fmt.Errorf("ingestion '%s' failed: %w", name, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("ingestion '%s' exited with code %d", name, exitCode)
	}
	fmt.Printf("✅ Ingestion '%s' completed\n", name)
	return nil
}

// checkComposeHealth waits briefly then prints the status of each compose service.
// Services that exited or are restarting are flagged so the user knows immediately.
func checkComposeHealth(projectName string) {
	time.Sleep(3 * time.Second)

	out, err := exec.Command("docker", "compose", "-p", projectName, "ps", "--format", "{{.Name}}\t{{.State}}\t{{.Status}}").Output() //nolint:gosec
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		name, state := parts[0], strings.ToLower(parts[1])
		status := ""
		if len(parts) == 3 {
			status = parts[2]
		}

		switch state {
		case "running":
			fmt.Printf("  %s✓%s %s %s(%s)%s\n", colorGreen, colorReset, name, colorDim, status, colorReset)
		case "exited", "dead":
			fmt.Printf("  %s✗%s %s %s— %s%s\n", colorRed, colorReset, name, colorRed, status, colorReset)
		default:
			fmt.Printf("  %s?%s %s %s(%s)%s\n", colorYellow, colorReset, name, colorDim, status, colorReset)
		}
	}
	fmt.Println()
}

// killProcessGroup sends SIGKILL to the entire process group of cmd.
// Falls back to killing just the process if the group kill fails.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}

// devLogsArgs returns the docker compose logs command arguments.
// Defaults to the agent service; --all tails everything; a specific service overrides both.
func devLogsArgs(composePath string, args []string, all bool) []string {
	logsArgs := []string{"compose", "-f", composePath, "logs", "-f"}
	if len(args) > 0 {
		logsArgs = append(logsArgs, args[0])
	} else if !all {
		logsArgs = append(logsArgs, "agent")
	}
	return logsArgs
}

// localDockerImage describes a Docker image to build in --local mode.
type localDockerImage struct {
	tag        string // e.g. "messaging:latest"
	dockerfile string // relative to ASTRO_ROOT, e.g. "modules/messaging/Dockerfile"
	context    string // relative to ASTRO_ROOT, e.g. "modules/messaging"
}

// devLocalImages are the infrastructure images built during `ast dev --local`.
var devLocalImages = []localDockerImage{
	{"messaging:latest", "modules/messaging/Dockerfile", "modules/messaging"},
	{"playground:latest", "modules/playground/Dockerfile", "modules/playground"},
}

// pushLocalInfraImages are the infrastructure images built during `ast push --local`.
// Collector is included because K8s deploys it as a sidecar; playground is omitted
// because it only runs in compose/dev mode.
var pushLocalInfraImages = []localDockerImage{
	{"messaging:latest", "modules/messaging/Dockerfile", "modules/messaging"},
	{"prod-astro-collector:latest", "deployment/Dockerfile.astro-collector", "."},
}

// buildLocalImages builds the given Docker images from ASTRO_ROOT source.
// Docker layer caching keeps repeat builds fast.
func buildLocalImages(images []localDockerImage, rebuild bool) error {
	astroRoot := os.Getenv("ASTRO_ROOT")
	if astroRoot == "" {
		return fmt.Errorf("ASTRO_ROOT is not set")
	}

	for _, img := range images {
		fmt.Printf("%s→%s Building %s...\n", colorCyan, colorReset, img.tag)
		dockerfile := filepath.Join(astroRoot, img.dockerfile)
		ctx := filepath.Join(astroRoot, img.context)
		args := []string{"build", "-t", img.tag, "-f", dockerfile}
		if rebuild {
			args = append(args, "--no-cache")
		}
		args = append(args, ctx)
		buildCmd := exec.Command("docker", args...) //nolint:gosec
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build %s: %w", img.tag, err)
		}
	}
	return nil
}

// composeBuildArgs returns the docker compose build command arguments.
// --pull is a boolean flag: true = always pull base images, false = use cache.
func composeBuildArgs(composePath string, rebuild, noPull bool) []string {
	args := []string{"compose", "--profile", "ingestion", "-f", composePath, "build"}
	if rebuild {
		args = append(args, "--no-cache")
	}
	if noPull {
		args = append(args, "--pull=false")
	} else {
		args = append(args, "--pull")
	}
	return args
}

// resolveAstroSourceRoot returns the Astro monorepo root from ASTRO_ROOT.
// Used in --local to link @saswatds/* from packages/.
func resolveAstroSourceRoot() (string, error) {
	p := os.Getenv("ASTRO_ROOT")
	if p == "" {
		return "", fmt.Errorf("ASTRO_ROOT is not set (required for --local to use local packages)\n\n  Set it to the path of your astro monorepo, e.g.:\n    export ASTRO_ROOT=$HOME/astro/astro")
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
	{"@astropods", "messaging", "modules/messaging/sdk/node"},
	{"@astropods", "adapter-core", "modules/adapters/packages/core"},
	{"@astropods", "adapter-mastra", "modules/adapters/packages/mastra"},
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
// Merges OS env, .env vars, and spec-resolved variables (provider credentials, model
// connection strings, inputs) so the local process sees the same env as the container.
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

	// Apply the same spec-resolved variables that buildEnvironment injects into
	// the Docker agent container (provider credentials, model URLs, inputs, etc.).
	// Container-based service names are rewritten to localhost since in --local
	// mode ports are published to the host.
	specEnv := composeBuilder.BuildEnvironment(s, envVars)
	for k, v := range specEnv {
		if v != nil {
			envMap[k] = *v
		}
	}

	rewriteDockerHostsToLocalhost(s, envMap)

	if s.Dev.HasMessagingAdapters() {
		envMap["GRPC_SERVER_ADDR"] = "localhost:9090"
	}
	envMap["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://localhost:4318"
	out := make([]string, 0, len(envMap))
	for k, v := range envMap {
		out = append(out, k+"="+v)
	}
	return out
}

// rewriteDockerHostsToLocalhost rewrites Docker-internal service hostnames to
// localhost in the agent env map. In --local mode the agent runs on the host,
// so Docker service names (model-*, knowledge-*, tool-*) won't resolve.
// Containers publish their ports to the host, so localhost works instead.
func rewriteDockerHostsToLocalhost(s *spec.AstroSpec, envMap map[string]string) {
	serviceNames := make(map[string]bool)
	for name, m := range s.Models {
		if m.DeploysContainer(s.Providers) {
			serviceNames[fmt.Sprintf("model-%s", name)] = true
		}
	}
	for name, k := range s.Knowledge {
		if k.DeploysContainer(s.Providers) {
			serviceNames[fmt.Sprintf("knowledge-%s", name)] = true
		}
	}
	for name, t := range s.Tools {
		if t.DeploysContainer(s.Providers) {
			serviceNames[fmt.Sprintf("tool-%s", name)] = true
		}
	}

	for k, v := range envMap {
		for svc := range serviceNames {
			if v == svc {
				envMap[k] = "localhost"
				break
			}
			if replaced := strings.ReplaceAll(v, svc, "localhost"); replaced != v {
				envMap[k] = replaced
				break
			}
		}
	}
}

// runStartupIngestions runs each startup-type ingestion synchronously before the CLI exits.
func runStartupIngestions(s *spec.AstroSpec, project *composeTypes.Project, verbose bool) {
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "startup" {
			continue
		}
		startupSvc, err := newComposeService(verbose)
		if err != nil {
			fmt.Printf("❌ Failed to init compose service for ingestion '%s': %v\n", name, err)
			continue
		}
		var exitCode int
		if err := withSpinner(fmt.Sprintf("Running ingestion: %s...", name), fmt.Sprintf("Ingestion %s complete", name), verbose, func() error {
			var runErr error
			exitCode, runErr = startupSvc.RunOneOffContainer(context.Background(), project, api.RunOptions{
				Service:    fmt.Sprintf("ingestion-%s", name),
				AutoRemove: true,
				NoDeps:     true,
			})
			return runErr
		}); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Startup ingestion '%s' failed: %v\n", name, err)
		} else if exitCode != 0 {
			fmt.Fprintf(os.Stderr, "❌ Startup ingestion '%s' exited with code %d\n", name, exitCode)
		}
	}
}

// printReadyBlock renders the post-start summary using lipgloss.
func printReadyBlock(s *spec.AstroSpec, hasWebInterface bool) {
	primary := lipgloss.NewStyle().Foreground(theme.Primary)
	bold := lipgloss.NewStyle().Bold(true)
	boldPrimary := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	dim := lipgloss.NewStyle().Faint(true)

	var lines []string
	lines = append(lines, "✨ "+bold.Render(s.Name)+" is ready")
	lines = append(lines, "")

	if s.Agent.HasFrontend() {
		port := 3200
		lines = append(lines, primary.Render("➜")+"  "+boldPrimary.Render(fmt.Sprintf("http://localhost:%d", port))+"  "+dim.Render("(frontend)"))
	}

	if hasWebInterface {
		lines = append(lines, primary.Render("➜")+"  "+boldPrimary.Render("http://localhost:3000"))
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
	lines = append(lines, bold.Render(binaryName+" dev logs")+"  — tail logs")
	lines = append(lines, bold.Render(binaryName+" dev stop")+"  — stop")

	// Manual / schedule ingestion hints
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type != "schedule" && ingestion.Trigger.Type != "manual" {
			continue
		}
		lines = append(lines, bold.Render(fmt.Sprintf("%s dev trigger %-8s", binaryName, name))+"— trigger ingestion")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Println()
	fmt.Println(box)
	fmt.Println()
}

// withSpinner runs fn with an animated spinner in non-verbose mode, or prints
// the title and runs fn directly in verbose mode.
func withSpinner(title, doneMsg string, verbose bool, fn func() error) error {
	if verbose {
		fmt.Println(title)
		return fn()
	}
	err := spinner.New().
		Title(" " + title + " (this may take a moment, use -v for verbose)").
		ActionWithErr(func(_ context.Context) error {
			return fn()
		}).
		Run()
	if err == nil {
		fmt.Printf("✅ %s\n", doneMsg)
	}
	return err
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

// hasOllamaModels returns true if any model in the spec uses the ollama provider.
func hasOllamaModels(s *spec.AstroSpec) bool {
	for _, model := range s.Models {
		if model.Provider == "ollama" {
			return true
		}
	}
	return false
}

const ollamaBaseURL = "http://localhost:11434"

// checkNativeOllama verifies that Ollama is installed and the server is reachable.
// Returns a user-friendly error with install/start instructions on failure.
func checkNativeOllama() error {
	// Try the API first — if it responds, Ollama is running regardless of binary location
	resp, err := http.Get(ollamaBaseURL + "/api/tags") //nolint:gosec,noctx
	if err == nil {
		resp.Body.Close() //nolint:errcheck,gosec
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}

	// API not reachable — check if the binary is installed to give the right error
	if _, lookErr := exec.LookPath("ollama"); lookErr != nil {
		red := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
		dim := lipgloss.NewStyle().Faint(true)
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

		msg := red.Render("Ollama is not installed") + "\n" +
			dim.Render("Your spec uses Ollama models, which require a local Ollama installation.") + "\n\n"
		switch runtime.GOOS {
		case "darwin":
			msg += hint.Render("  Install with Homebrew:") + "\n" +
				"    brew install ollama\n\n" +
				hint.Render("  Or download the desktop app:") + "\n" +
				"    https://ollama.com/download/mac"
		case "linux":
			msg += hint.Render("  Install with the official script:") + "\n" +
				"    curl -fsSL https://ollama.com/install.sh | sh\n\n" +
				hint.Render("  Or visit:") + "\n" +
				"    https://ollama.com/download/linux"
		default:
			msg += hint.Render("  Download from:") + "\n" +
				"    https://ollama.com/download"
		}
		return fmt.Errorf("%s", msg)
	}

	// Binary exists but server isn't responding
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	msg := red.Render("Ollama is not running") + "\n" +
		dim.Render("Ollama is installed but the server isn't responding.") + "\n\n"
	switch runtime.GOOS {
	case "darwin":
		msg += hint.Render("  Start in foreground:") + "\n" +
			"    ollama serve\n\n" +
			hint.Render("  Or as a background service:") + "\n" +
			"    brew services start ollama"
	case "linux":
		msg += hint.Render("  Start with:") + "\n" +
			"    ollama serve\n\n" +
			hint.Render("  Or as a systemd service:") + "\n" +
			"    sudo systemctl start ollama"
	default:
		msg += hint.Render("  Start with:") + "\n" +
			"    ollama serve"
	}
	return fmt.Errorf("%s", msg)
}

// ollamaLocalModels returns the set of model names available locally via the Ollama API.
func ollamaLocalModels() map[string]bool {
	resp, err := http.Get(ollamaBaseURL + "/api/tags") //nolint:gosec,noctx
	if err != nil {
		return nil
	}
	defer resp.Body.Close() //nolint:errcheck,gosec
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	models := make(map[string]bool, len(result.Models))
	for _, m := range result.Models {
		models[m.Name] = true
		// Also index without :latest so "llama3.2:1b" matches "llama3.2:1b:latest"
		if strings.HasSuffix(m.Name, ":latest") {
			models[strings.TrimSuffix(m.Name, ":latest")] = true
		}
	}
	return models
}

// ollamaPullModel pulls a model using the Ollama API with streaming progress.
func ollamaPullModel(name string) error {
	body, err := json.Marshal(map[string]any{"name": name, "stream": true})
	if err != nil {
		return err
	}
	resp, err := http.Post(ollamaBaseURL+"/api/pull", "application/json", bytes.NewReader(body)) //nolint:gosec,noctx
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck,gosec
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama pull failed (%d): %s", resp.StatusCode, string(respBody))
	}

	// Stream pull progress
	decoder := json.NewDecoder(resp.Body)
	var lastStatus string
	for decoder.More() {
		var event struct {
			Status    string `json:"status"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if err := decoder.Decode(&event); err != nil {
			break
		}
		if event.Error != "" {
			return fmt.Errorf("%s", event.Error)
		}
		if event.Total > 0 && event.Completed > 0 {
			pct := float64(event.Completed) / float64(event.Total) * 100
			fmt.Printf("\r   %s %.0f%%", event.Status, pct)
		} else if event.Status != lastStatus {
			if lastStatus != "" {
				fmt.Println()
			}
			fmt.Printf("   %s", event.Status)
			lastStatus = event.Status
		}
	}
	fmt.Println()
	return nil
}

// ensureOllamaModels pulls all required Ollama models on the host if not already present.
// Before pulling, it checks if the model's estimated size fits the system's available RAM.
func ensureOllamaModels(s *spec.AstroSpec, verbose bool) error {
	systemRAM := getSystemRAMBytes()
	localModels := ollamaLocalModels()

	for _, model := range s.Models {
		if model.Provider != "ollama" {
			continue
		}
		for _, m := range model.ResolvedModels() {
			if localModels[m] {
				fmt.Printf("%s→%s Model %s already available\n", colorCyan, colorReset, m)
				continue
			}

			// Warn if the model is likely too large for this system
			if systemRAM > 0 {
				if err := warnIfModelTooLarge(m, systemRAM); err != nil {
					return err
				}
			}

			fmt.Printf("%s→%s Pulling model %s...\n", colorCyan, colorReset, m)
			if err := ollamaPullModel(m); err != nil {
				return fmt.Errorf("failed to pull model %s: %w", m, err)
			}
			fmt.Printf("%s→%s Model %s ready\n", colorCyan, colorReset, m)
		}
	}
	return nil
}

// paramSizeRegex extracts the parameter count from a model tag like "llama3.3:70b" or "mistral:7b".
var paramSizeRegex = regexp.MustCompile(`(\d+(?:\.\d+)?)b`)

// estimateModelRAM returns the approximate RAM (in bytes) a model needs based on its parameter count.
// Rule of thumb: ~0.5–0.6 bytes per parameter for Q4 quantized models (Ollama default).
// We use 0.6 as a conservative estimate, plus ~1GB overhead for KV cache and runtime.
func estimateModelRAM(modelName string) uint64 {
	// Extract parameter count from model name/tag (e.g. "70b" from "llama3.3:70b")
	parts := strings.Split(modelName, ":")
	searchStr := modelName
	if len(parts) > 1 {
		searchStr = parts[1] // search the tag portion
	}
	matches := paramSizeRegex.FindStringSubmatch(searchStr)
	if len(matches) < 2 {
		// Also try the model name portion for patterns like "orca-mini:3b"
		matches = paramSizeRegex.FindStringSubmatch(parts[0])
	}
	if len(matches) < 2 {
		return 0 // can't estimate
	}
	params, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}
	// 0.6 bytes per param (Q4 quant) + 1GB overhead
	return uint64(params*0.6*1e9) + 1<<30
}

// warnIfModelTooLarge checks estimated model RAM against system RAM and prompts the user.
func warnIfModelTooLarge(modelName string, systemRAM uint64) error {
	estimated := estimateModelRAM(modelName)
	if estimated == 0 || estimated <= systemRAM {
		return nil
	}

	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	fmt.Println()
	fmt.Println(yellow.Render(fmt.Sprintf("  Warning: %s requires ~%dGB RAM but this system has %dGB",
		modelName, estimated>>30, systemRAM>>30)))
	fmt.Println(dim.Render("  The model may run very slowly or fail to load."))
	fmt.Println()

	var confirm bool
	if err := huh.NewConfirm().
		Title("Continue pulling this model?").
		Affirmative("Yes, pull anyway").
		Negative("No, abort").
		Value(&confirm).
		Run(); err != nil {
		return fmt.Errorf("aborted")
	}
	if !confirm {
		return fmt.Errorf("aborted: model %s is too large for this system", modelName)
	}
	return nil
}

// getSystemRAMBytes returns total physical RAM in bytes, or 0 if unknown.
func getSystemRAMBytes() uint64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		val, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0
		}
		return val
	case "linux":
		out, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					kb, err := strconv.ParseUint(fields[1], 10, 64)
					if err == nil {
						return kb * 1024
					}
				}
			}
		}
		return 0
	default:
		return 0
	}
}
