package cmd

import (
	"context"
	"fmt"
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
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/config"
	"github.com/astropods/astro-cli/internal/theme"
	"github.com/astropods/astro-cli/internal/utils"
	spec "github.com/astropods/astro-spec"
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
	// Build Docker Compose project
	project, err := composeBuilder.BuildProject(astroSpec, workingDir, envVars)
	if err != nil {
		return fmt.Errorf("failed to build compose project: %w", err)
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
	if noPull {
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

	// Serve the chat UI from the CLI, proxying to the local messaging
	// sidecar. Runs as a detached worker so it survives background mode;
	// in foreground it stops on teardown, in background it outlives the CLI.
	startChatUI(astDir, astroSpec.Name, hasWebInterface, !background)

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
	stopChatUI(astDir)
	_ = os.Remove(filepath.Join(astDir, ".running"))
	fmt.Printf("%s→%s Stopped\n", colorCyan, colorReset)
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
	stopChatUI(filepath.Dir(statePath))
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
		lines = append(lines, primary.Render("➜")+"  "+boldPrimary.Render(chatUIURL)+"  "+dim.Render("(chat)"))
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
