package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
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
	specFile, _ := cmd.Flags().GetString("file")
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

	if verbose {
		fmt.Printf("   Services: %d\n", len(project.Services))
		for name := range project.Services {
			fmt.Printf("     - %s\n", name)
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

	// Start services using Docker Compose CLI
	fmt.Println("🔨 Building and starting services...")

	// Build with or without cache based on rebuild flag
	if rebuild {
		fmt.Println("   Using --no-cache for clean rebuild...")
		buildCmd := exec.Command("docker", "compose", "-f", cPath, "build", "--no-cache") //nolint:gosec
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build services: %w", err)
		}
	}

	upArgs := []string{"compose", "-f", cPath, "up", "-d", "--build"}
	if noPull {
		upArgs = append(upArgs, "--pull=never")
	}
	upCmd := exec.Command("docker", upArgs...) //nolint:gosec
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	fmt.Println()
	fmt.Println("✅ All services running!")

	// Check if messaging interface is configured (from dev section)
	hasMessagingInterface := false
	hasWebInterface := false
	if astroSpec.Dev != nil {
		for _, name := range astroSpec.Dev.Interfaces {
			if name == "slack" || name == "web" {
				hasMessagingInterface = true
			}
			if name == "web" {
				hasWebInterface = true
			}
		}
	}

	if hasMessagingInterface && !hasWebInterface {
		fmt.Println("💬 Messaging service running on gRPC port 9090")
	}

	// --local: run agent as local process and block
	if local {
		return runLocalAgent(cmd, astroSpec, workingDir, cPath, envVars, hasWebInterface)
	}

	// Non-local mode: print hints and exit
	fmt.Println()
	if hasWebInterface {
		fmt.Println("  Your agent is ready. Open the playground to start chatting:")
		fmt.Println()
		fmt.Printf("  %s%s➜  http://localhost:3000%s\n", colorBold, colorGreen, colorReset)
		fmt.Println()
		fmt.Printf("  %sAPI  http://localhost:3100%s\n", colorDim, colorReset)
	}
	fmt.Println()
	fmt.Printf("  %sast dev logs%s  — tail logs\n", colorBold, colorReset)
	fmt.Printf("  %sast dev stop%s  — stop\n", colorBold, colorReset)
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
	msgSDK := filepath.Join(astroRoot, "packages", "astro-messaging", "sdk", "node")
	if _, err := os.Stat(filepath.Join(msgSDK, "dist", "index.js")); err != nil {
		agentCancel()
		return fmt.Errorf("messaging SDK not built: run 'cd %s && bun run build' first", msgSDK)
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

	// Set up cron scheduler for ingestion workers
	var cronScheduler *cron.Cron
	if len(astroSpec.Ingestion) > 0 {
		cronScheduler = cron.New()

		for name, ingestion := range astroSpec.Ingestion {
			devSchedule := ""
			if astroSpec.Dev != nil {
				devSchedule = astroSpec.Dev.Schedules[name]
			}
			if ingestion.Trigger.Type == "schedule" && devSchedule != "" {
				cronPattern := devSchedule
				ingestionName := name

				fmt.Printf("⏰ Scheduling ingestion '%s' with pattern: %s\n", ingestionName, cronPattern)

				_, err := cronScheduler.AddFunc(cronPattern, func() {
					fmt.Printf("🔄 Running ingestion: %s\n", ingestionName)

					serviceName := fmt.Sprintf("ingestion-%s", ingestionName)
					runCmd := exec.Command("docker", "compose", "-f", cPath, "run", "--rm", serviceName) //nolint:gosec
					runCmd.Stdout = os.Stdout
					runCmd.Stderr = os.Stderr

					if err := runCmd.Run(); err != nil {
						fmt.Printf("❌ Failed to run ingestion '%s': %v\n", ingestionName, err)
					} else {
						fmt.Printf("✅ Ingestion '%s' completed\n", ingestionName)
					}
				})

				if err != nil {
					fmt.Printf("⚠️  Failed to schedule ingestion '%s': %v\n", ingestionName, err)
				}
			} else if ingestion.Trigger.Type == "startup" {
				ingestionName := name
				fmt.Printf("🚀 Running startup ingestion: %s\n", ingestionName)

				serviceName := fmt.Sprintf("ingestion-%s", ingestionName)
				go func() {
					runCmd := exec.Command("docker", "compose", "-f", cPath, "run", "--rm", serviceName) //nolint:gosec
					runCmd.Stdout = os.Stdout
					runCmd.Stderr = os.Stderr
					if err := runCmd.Run(); err != nil {
						fmt.Printf("❌ Failed to run startup ingestion '%s': %v\n", ingestionName, err)
					} else {
						fmt.Printf("✅ Startup ingestion '%s' completed\n", ingestionName)
					}
				}()
			}
		}

		if cronScheduler != nil {
			cronScheduler.Start()
			defer cronScheduler.Stop()
			fmt.Println("📅 Ingestion scheduler started")
		}
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
	scope string // e.g. "@saswatds" or "@astropods"
	name  string // e.g. "astro-agent"
	path  string // relative to astroRoot, e.g. "packages/astro-agent"
}

// localAstroPackages are the packages we link in --local and remove in --local-reset.
var localAstroPackages = []localPackage{
	{"@saswatds", "astro-agent", "packages/astro-agent"},
	{"@saswatds", "astro-graph", "packages/astro-graph"},
	{"@astropods", "messaging", "packages/astro-messaging/sdk/node"},
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
