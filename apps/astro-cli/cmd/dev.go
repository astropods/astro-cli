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
	"github.com/charmbracelet/lipgloss"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	composeBuilder "github.com/astropods/astro/apps/astro-cli/internal/compose"
	"github.com/astropods/astro/apps/astro-cli/internal/config"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	"github.com/astropods/astro/apps/astro-cli/internal/utils"
	spec "github.com/astropods/astro/packages/astro-spec"
)

var devCmd = &cobra.Command{
	Use:     "project",
	Aliases: []string{"dev"},
	Short:   "Manage local project development environment",
	Args:    cobra.NoArgs,
	RunE:    runDevStart,
}

var devStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start project containers",
	Args:  cobra.NoArgs,
	RunE:  runDevStart,
}

var devLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "Tail container logs",
	Long: `Tail logs from the running project containers.

Defaults to the agent container. Use --all to tail all services.
Optionally specify a service name (e.g. astro-messaging) to tail a specific container.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDevLogs,
}

var devStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop project containers",
	Long:  `Stop and remove the running projects containers.`,
	Args:  cobra.NoArgs,
	RunE:  runDevStop,
}

var devTriggerCmd = &cobra.Command{
	Use:   "trigger <name>",
	Short: "Trigger an ingestion job",
	Long:  `Manually trigger a named ingestion job. Runs the ingestion container and exits when done.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDevTrigger,
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devStartCmd)
	devCmd.AddCommand(devLogsCmd)
	devCmd.AddCommand(devStopCmd)
	devCmd.AddCommand(devTriggerCmd)

	devStartCmd.Long = `Start the local development environment with Docker containers.

By default, tails the agent service's logs in the foreground and stops
containers on Ctrl+C. Sidecar logs (models, knowledge stores, integrations,
messaging) are suppressed — use 'project logs <service>' to view them on
demand, or --all-logs to tail every service.

Use -b/--background to start in the background and exit immediately.`

	// Flags on both devCmd and devStartCmd
	for _, cmd := range []*cobra.Command{devCmd, devStartCmd} {
		cmd.Flags().String("env", utils.DefaultEnvFile, "Environment file for integration credentials")
		cmd.Flags().Bool("rebuild", false, "Force rebuild all containers without cache")
		cmd.Flags().Bool("no-pull", false, "Skip pulling images (use only locally built images)")
		cmd.Flags().BoolP("background", "b", false, "Start containers in the background and exit (use 'project logs' / 'project stop' to manage)")
		cmd.Flags().Bool("all-logs", false, "Tail logs from every service instead of just the agent")
		cmd.Flags().Bool("local", false, "Use local images, no pull, run agent as local process (bun for ts, python3 for py); implies --no-pull")
		cmd.Flags().Bool("local-reset", false, fmt.Sprintf("Remove local packages injected by --local (use after %s project start --local); run 'bun install' (ts) or 'pip install -r requirements.txt' (py) to restore deps", buildinfo.BinaryName))
		_ = cmd.Flags().MarkHidden("local")
		_ = cmd.Flags().MarkHidden("local-reset")
	}

	devLogsCmd.Flags().Bool("all", false, "Tail logs from all services (not just agent)")
}

// checkDockerRunning verifies the Docker daemon is accessible.
func checkDockerRunning() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("Windows is not supported — please use macOS or Linux") //nolint:staticcheck
	}
	_, err := newDockerClient()
	return err
}

// devStatePath returns the path to the dev-environment marker file.
func devStatePath() (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return filepath.Join(workingDir, buildinfo.AppDirName, ".running"), nil
}

// readDevProjectName returns the compose project name stored in the .running
// marker file. The returned value is always normalized through
// composeBuilder.ProjectNameFromSpecName so legacy state files containing
// scoped spec names (e.g. "@org/my-agent") still map to the live project
// ("my-agent") used by Up — avoiding mismatched Down/Logs/stop calls.
// Falls back to spec parsing if the file is empty (older format).
func readDevProjectName(statePath string, cmd *cobra.Command) (string, error) {
	data, err := os.ReadFile(statePath) //nolint:gosec // path is constructed from os.Getwd() + hardcoded suffix
	if err != nil {
		return "", fmt.Errorf("failed to read dev state: %w", err)
	}
	if name := strings.TrimSpace(string(data)); name != "" {
		return composeBuilder.ProjectNameFromSpecName(name), nil
	}
	// Fallback: parse spec for the project name
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	specPath, err := resolveSpecPath(flagString(cmd, "file"), workingDir)
	if err != nil {
		return "", err
	}
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse spec: %w", err)
	}
	return composeBuilder.ProjectName(astroSpec), nil
}

func runDevStart(cmd *cobra.Command, args []string) error {
	envFile := flagString(cmd, "env")
	rebuild := flagBool(cmd, "rebuild")
	noPull := flagBool(cmd, "no-pull")
	local := flagBool(cmd, "local")
	localReset := flagBool(cmd, "local-reset")
	background := flagBool(cmd, "background")
	allLogs := flagBool(cmd, "all-logs")

	if err := checkDockerRunning(); err != nil {
		return err
	}

	verbose, _ := cmd.Flags().GetBool("verbose")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
	if err != nil {
		return err
	}

	_, statErr := os.Stat(filepath.Join(workingDir, "requirements.txt"))
	isPython := statErr == nil
	isTypeScript := !isPython

	if localReset {
		if isTypeScript {
			if err := unlinkLocalPackages(workingDir); err != nil {
				return fmt.Errorf("local-reset: %w", err)
			}
			fmt.Printf("%s→%s Removed local packages. Run 'bun install' to restore dependencies.\n", colorCyan, colorReset)
		} else if isPython {
			if err := uninstallLocalPythonPackages(workingDir); err != nil {
				return fmt.Errorf("local-reset: %w", err)
			}
			fmt.Printf("%s→%s Removed local Python packages. Restored from requirements.txt.\n", colorCyan, colorReset)
		}
		return nil
	}

	// --local requires ASTRO_ROOT for local packages
	if local {
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
	storedVars := config.GetProjectVars(buildinfo.BinaryName, workingDir)
	for k, v := range storedVars {
		envVars[k] = v
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("failed to set env var %s: %w", k, err)
		}
	}
	if len(storedVars) > 0 {
		fmt.Printf("%s→%s Config: %d variable(s) from project store\n", colorCyan, colorReset, len(storedVars))
	} else if len(envVars) == 0 {
		fmt.Printf("%s→%s %sNo credentials found. Run '%s configure' to set up.%s\n", colorCyan, colorReset, colorDim, buildinfo.BinaryName, colorReset)
	}

	// AI Gateway: if the spec uses provider:astro-gateway, fetch a short-lived
	// dev key from astro-server and inject the resolver-derived env vars into
	// the local container env. The key auto-expires upstream — no cleanup
	// needed on stop.
	if specUsesAIGateway(astroSpec) {
		at, atErr := getCurrentAccountToken(cmd.Context())
		if atErr != nil {
			return fmt.Errorf("provider:astro-gateway requires login — run '%s login': %w", buildinfo.BinaryName, atErr)
		}
		keyResp, keyErr := fetchAIGatewayDevKey(cmd.Context(), at, astroSpec, verbose)
		if keyErr != nil {
			return keyErr
		}
		if err := applyAIGatewayDevKey(astroSpec, keyResp, envVars); err != nil {
			return err
		}
		fmt.Printf("%s→%s AI Gateway: dev key minted (expires %s)\n", colorCyan, colorReset, keyResp.ExpiresAt)
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

	// --local: omit agent from compose and run it as a local process
	if local {
		delete(project.Services, "agent")
		if verbose {
			fmt.Println("   --local: agent will run as local process")
		}
	}

	astDir := filepath.Join(workingDir, buildinfo.AppDirName)
	if err := os.MkdirAll(astDir, 0755); err != nil { //nolint:gosec
		return fmt.Errorf("failed to create %s directory: %w", buildinfo.AppDirName, err)
	}

	// Log services before building
	serviceNames := make([]string, 0, len(project.Services))
	for name := range project.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	fmt.Printf("%s→%s Services: %s\n", colorCyan, colorReset, strings.Join(serviceNames, ", "))

	svc, err := newComposeService(verbose)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}

	// Build all services upfront — including profiled ingestion containers — so
	// startup ingestions don't get built lazily after everything else is running.
	// Activate the "ingestion" profile so the SDK resolves profiled services by name.
	buildProject := *project
	buildProject.Profiles = []string{"ingestion"}
	buildTitle := "Building services..."
	if rebuild {
		buildTitle = "Building services (no cache)..."
	}
	if err := withSpinner(buildTitle, "Build complete", verbose, func() error {
		return svc.Build(context.Background(), &buildProject, api.BuildOptions{
			NoCache:  rebuild,
			Quiet:    !verbose,
			Services: allServiceNames(project),
		})
	}); err != nil {
		return fmt.Errorf("failed to build services: %w", err)
	}

	// Tear down leftover containers from a previous run (e.g. force-killed with Ctrl+C).
	// This is fast and idempotent when nothing is running. Must match the
	// project name produced by BuildProject (via composeBuilder.ProjectName),
	// otherwise compose finds no resources labelled with the raw spec name
	// and prints "No resource found to remove" for scoped agents.
	projectName := composeBuilder.ProjectName(astroSpec)
	_ = svc.Down(context.Background(), projectName, api.DownOptions{RemoveOrphans: true})

	// Start non-profiled services (already built above)
	upProject := projectForUp(project)
	if noPull || local {
		for name, svc := range upProject.Services {
			svc.PullPolicy = composeTypes.PullPolicyNever
			upProject.Services[name] = svc
		}
	}
	if err := withSpinner("Starting services...", "Services started", verbose, func() error {
		return svc.Up(context.Background(), upProject, api.UpOptions{
			Create: api.CreateOptions{
				Build:         nil,  // --no-build (already built above)
				RemoveOrphans: true, // remove stale containers from previous runs
			},
			Start: api.StartOptions{Project: upProject}, // pass project to skip container label lookup
		})
	}); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	// Write marker so subcommands (logs, stop, trigger) know dev is running.
	// The file contains the compose project name so subcommands can avoid
	// re-parsing the spec and match the project used by Up.
	if err := os.WriteFile(filepath.Join(astDir, ".running"), []byte(projectName), 0644); err != nil { //nolint:gosec
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
		return runLocalAgent(cmd, astroSpec, projectName, workingDir, envVars, hasWebInterface, allLogs)
	}

	// Run startup ingestions before printing the ready block so output isn't interleaved
	runStartupIngestions(astroSpec, project, verbose)

	printReadyBlock(astroSpec, hasWebInterface, background)

	if background {
		return nil
	}
	return runForeground(projectName, astDir, allLogs)
}

// runForeground tails the agent's container logs and blocks until Ctrl+C,
// then stops. Sidecar services run silently — fetch their logs on demand via
// `project logs <service>`. Pass allLogs=true to tail every service.
func runForeground(projectName, astDir string, allLogs bool) error {
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

	logOpts := api.LogOptions{Follow: true}
	if !allLogs {
		logOpts.Services = []string{"agent"}
	}
	_ = logsSvc.Logs(logsCtx, projectName, &stdoutLogConsumer{out: os.Stdout, err: os.Stderr}, logOpts)
	logsCancel()
	signal.Stop(sigChan)

	fmt.Println()
	fmt.Printf("%s→%s Stopping containers...\n", colorCyan, colorReset)
	stopSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}
	_ = stopSvc.Down(context.Background(), projectName, api.DownOptions{RemoveOrphans: true})
	_ = os.Remove(filepath.Join(astDir, ".running"))
	fmt.Printf("%s→%s Stopped\n", colorCyan, colorReset)
	return nil
}

// runLocalAgent runs the agent as a local bun process and blocks until Ctrl+C.
// projectName is the compose project name computed by composeBuilder.ProjectName,
// used for Logs/health/Down so they match the project name used by Up.
// allLogs=false suppresses the background sidecar tail since the agent's own
// stdout already streams to this terminal; pass true to surface sidecar
// failures live alongside the agent.
func runLocalAgent(_ *cobra.Command, astroSpec *spec.AstroSpec, projectName string, workingDir string, envVars map[string]string, hasWebInterface bool, allLogs bool) error {
	agentCtx, agentCancel := context.WithCancel(context.Background())
	agentEnv := buildLocalAgentEnv(astroSpec, envVars)

	// Use local @astropods/* packages from ASTRO_ROOT
	astroRoot, err := resolveAstroSourceRoot()
	if err != nil {
		agentCancel()
		return err
	}

	_, statErr := os.Stat(filepath.Join(workingDir, "requirements.txt"))
	isPython := statErr == nil
	isTypeScript := !isPython
	if isTypeScript {
		// TypeScript agent: symlink @astropods/* packages and build SDKs.
		if err := linkLocalPackages(workingDir, astroRoot); err != nil {
			agentCancel()
			return fmt.Errorf("link local packages: %w", err)
		}
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
	} else if isPython {
		// Python agent: pip install -e local packages so the agent uses local source.
		fmt.Printf("%s→%s Using local Python packages from %s\n", colorCyan, colorReset, astroRoot)
		if err := installLocalPythonPackages(workingDir, astroRoot); err != nil {
			agentCancel()
			return err
		}
	}

	// Resolve start command from spec. Default differs by language.
	startCommand := "bun --watch run start"
	if isPython {
		startCommand = "python3 -m agent.main"
	}
	if astroSpec.Dev != nil && astroSpec.Dev.Command != "" {
		startCommand = astroSpec.Dev.Command
	}

	// Run via shell so the command string is interpreted correctly.
	// Setpgid gives the process its own group so we can kill the entire tree
	// (bun --watch / python3 plus any agent-spawned children) via the group pid.
	agentCmd := exec.CommandContext(agentCtx, "sh", "-c", startCommand) //nolint:gosec
	agentCmd.Dir = workingDir
	agentCmd.Env = agentEnv
	agentCmd.Stdout = os.Stdout
	agentCmd.Stderr = os.Stderr
	agentCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Override the default context-cancel kill (Process.Kill, which only targets sh)
	// to signal the whole process group so watchers and grandchildren don't leak.
	agentCmd.Cancel = func() error {
		if agentCmd.Process == nil {
			return nil
		}
		return syscall.Kill(-agentCmd.Process.Pid, syscall.SIGTERM)
	}
	if err := agentCmd.Start(); err != nil {
		agentCancel()
		return fmt.Errorf("failed to start agent: %w", err)
	}
	fmt.Printf("%s→%s Agent running as local process %s(%s)%s\n", colorCyan, colorReset, colorDim, startCommand, colorReset)

	checkComposeHealth(projectName)

	// Stream docker compose logs only when --all-logs is set. In the default
	// --local flow the agent runs as a host process and already streams its
	// own stdout/stderr to this terminal; tailing sidecars on top of that
	// drowns the agent's own output in noise. Sidecar logs remain available
	// via `project logs <service>`.
	logsCtx, logsCancel := context.WithCancel(context.Background())
	if allLogs {
		logsSvc, err := newComposeService(false)
		if err != nil {
			logsCancel()
			agentCancel()
			return fmt.Errorf("failed to init compose service for logs: %w", err)
		}
		go func() { //nolint:errcheck
			_ = logsSvc.Logs(logsCtx, projectName, &stdoutLogConsumer{out: os.Stdout, err: os.Stderr},
				api.LogOptions{Follow: true})
		}()
	}

	if hasWebInterface {
		go func() {
			time.Sleep(2 * time.Second)
			openBrowser("http://localhost:3100")
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
	if agentCmd.Process != nil {
		// SIGTERM the entire process group (Setpgid above) so bun --watch
		// workers and any children the agent spawned exit too — not just sh.
		pgid := agentCmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = agentCmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-done
		}
	}
	agentCancel()

	// Stop all services
	localSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}
	if err := localSvc.Down(context.Background(), projectName, api.DownOptions{}); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	fmt.Printf("%s→%s Cleanup complete\n", colorCyan, colorReset)
	fmt.Printf("  %sTip: run '%s project start --local-reset' to remove injected local dependencies%s\n", colorDim, buildinfo.BinaryName, colorReset)

	return nil
}

func runDevLogs(cmd *cobra.Command, args []string) error {
	statePath, err := devStatePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("no dev environment running. Run '%s project start' first", buildinfo.BinaryName)
	}

	allLogs := flagBool(cmd, "all")
	service := "agent"
	if len(args) > 0 {
		service = args[0]
		allLogs = false // explicit service arg overrides --all
	}

	projectName, err := readDevProjectName(statePath, cmd)
	if err != nil {
		return err
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

	logOpts := api.LogOptions{Follow: true}
	if !allLogs {
		logOpts.Services = []string{service}
	}
	err = logsSvc.Logs(logsCtx, projectName, &stdoutLogConsumer{out: os.Stdout, err: os.Stderr}, logOpts)
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
		return fmt.Errorf("no dev environment running. Run '%s project start' first", buildinfo.BinaryName)
	}

	fmt.Println("🛑 Stopping dev containers...")
	projectName, err := readDevProjectName(statePath, cmd)
	if err != nil {
		return err
	}

	stopSvc, err := newComposeService(false)
	if err != nil {
		return fmt.Errorf("failed to init compose service: %w", err)
	}
	err = withSpinner("Stopping services...", "Services stopped", false, func() error {
		return stopSvc.Down(context.Background(), projectName, api.DownOptions{RemoveOrphans: true})
	})
	_ = os.Remove(statePath) // always remove — containers may be gone even on error
	if err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	fmt.Printf("%s→%s Containers stopped\n", colorCyan, colorReset)
	return nil
}

func runDevTrigger(cmd *cobra.Command, args []string) error {
	envFile := flagString(cmd, "env")

	if err := checkDockerRunning(); err != nil {
		return err
	}

	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
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
		fmt.Printf("Run %s%s project trigger <name>%s to trigger one.\n", colorBold, buildinfo.BinaryName, colorReset)
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
		return fmt.Errorf("no dev environment running. Run '%s project start' first", buildinfo.BinaryName)
	}

	fmt.Printf("🔄 Triggering ingestion: %s\n", name)
	envVars, err := utils.LoadEnvFile(workingDir, envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}
	if envVars == nil {
		envVars = make(map[string]string)
	}
	// Merge stored project vars (same as runDevStart — takes priority over .env)
	for k, v := range config.GetProjectVars(buildinfo.BinaryName, workingDir) {
		envVars[k] = v
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

	ctx := context.Background()
	svc, err := newComposeService(false)
	if err != nil {
		return
	}
	containers, err := svc.Ps(ctx, projectName, api.PsOptions{All: true})
	if err != nil {
		return
	}

	for _, c := range containers {
		switch string(c.State) {
		case "running":
			fmt.Printf("  %s✓%s %s %s(%s)%s\n", colorGreen, colorReset, c.Name, colorDim, c.Status, colorReset)
		case "exited", "dead":
			fmt.Printf("  %s✗%s %s %s— %s%s\n", colorRed, colorReset, c.Name, colorRed, c.Status, colorReset)
		default:
			fmt.Printf("  %s?%s %s %s(%s)%s\n", colorYellow, colorReset, c.Name, colorDim, c.Status, colorReset)
		}
	}
	fmt.Println()
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

// localPythonPackage describes a Python package to install in --local mode.
// Packages must be listed in dependency order (dependencies before dependents).
type localPythonPackage struct {
	name string // PyPI package name, e.g. "astropods-messaging"
	path string // relative to astroRoot, e.g. "modules/messaging/sdk/python"
}

// localAstroPythonPackages are the packages installed editable in --local and removed in --local-reset.
var localAstroPythonPackages = []localPythonPackage{
	{"astropods-messaging", "modules/messaging/sdk/python"},
	{"astropods-adapter-core", "modules/adapters/packages/core-py"},
	{"astropods-adapter-langchain", "modules/adapters/packages/langchain"},
}

// installLocalPythonPackages pip-installs each package as editable from ASTRO_ROOT source
// so the agent uses local source in --local mode.
func installLocalPythonPackages(workingDir, astroRoot string) error {
	for _, pkg := range localAstroPythonPackages {
		fmt.Printf("%s→%s Installing %s...\n", colorCyan, colorReset, pkg.name)
		pkgPath := filepath.Join(astroRoot, pkg.path)
		pipCmd := exec.Command("python3", "-m", "pip", "install", "-e", pkgPath, "--quiet") //nolint:gosec
		pipCmd.Dir = workingDir
		pipCmd.Stdout = os.Stdout
		pipCmd.Stderr = os.Stderr
		if err := pipCmd.Run(); err != nil {
			return fmt.Errorf("failed to install %s: %w", pkg.name, err)
		}
	}
	return nil
}

// uninstallLocalPythonPackages removes the editable installs and restores PyPI versions
// from requirements.txt so the user can work without ASTRO_ROOT.
func uninstallLocalPythonPackages(workingDir string) error {
	for _, pkg := range localAstroPythonPackages {
		pipCmd := exec.Command("python3", "-m", "pip", "uninstall", "-y", pkg.name) //nolint:gosec
		pipCmd.Dir = workingDir
		pipCmd.Stdout = os.Stdout
		pipCmd.Stderr = os.Stderr
		// Ignore errors — package may not be installed.
		_ = pipCmd.Run()
	}
	// Reinstall from requirements.txt to restore PyPI versions.
	pipCmd := exec.Command("python3", "-m", "pip", "install", "-r", "requirements.txt", "--quiet") //nolint:gosec
	pipCmd.Dir = workingDir
	pipCmd.Stdout = os.Stdout
	pipCmd.Stderr = os.Stderr
	return pipCmd.Run()
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
		// Keep this in sync with buildMessagingPorts host-published gRPC port.
		envMap["GRPC_SERVER_ADDR"] = "localhost:19090"
	}
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
	for name, t := range s.Integrations {
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
func printReadyBlock(s *spec.AstroSpec, hasWebInterface bool, background bool) {
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
		lines = append(lines, primary.Render("➜")+"  "+boldPrimary.Render("http://localhost:3100")+"  "+dim.Render("(playground)"))
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
	if background {
		lines = append(lines, bold.Render(buildinfo.BinaryName+" project logs")+"  — tail logs")
		lines = append(lines, bold.Render(buildinfo.BinaryName+" project stop")+"  — stop")
	} else {
		lines = append(lines, dim.Render("Ctrl+C to stop"))
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
		if err := fn(); err != nil {
			return err
		}
		fmt.Printf("✅ %s\n", doneMsg)
		return nil
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
func ensureOllamaModels(s *spec.AstroSpec, _ bool) error {
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
