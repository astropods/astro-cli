package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	composeBuilder "github.com/postman/astro/apps/astro-cli/internal/compose"
	"github.com/postman/astro/apps/astro-cli/internal/utils"
	"github.com/postman/astro/apps/astro-cli/internal/watcher"
	spec "github.com/postman/astro/packages/astro-spec"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Run agent locally with hot reload for development",
	Long: `Run the agent locally with self-hosted components in Docker containers.

The dev command:
1. Spins up self-hosted components (models, knowledge stores, tools)
2. Builds and runs the agent container
3. Watches for file changes and hot-reloads the agent
4. Injects credentials from .env file
5. Auto-configures component connection strings

Example:
  ast dev
  ast dev --file custom-astro.yml
  ast dev --env .env.local`,
	RunE: runDev,
}

var (
	envFile    string
	noReload   bool
	rebuild    bool
	noPull     bool
	local      bool
	localReset bool
)

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.Flags().StringVar(&envFile, "env", utils.DefaultEnvFile, "Environment file for integration credentials")
	devCmd.Flags().BoolVar(&noReload, "no-reload", false, "Disable hot reload")
	devCmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force rebuild all containers without cache")
	devCmd.Flags().BoolVar(&noPull, "no-pull", false, "Skip pulling images (use only locally built images)")
	devCmd.Flags().BoolVar(&local, "local", false, "Use local images, no pull, run agent as local process (bun); implies --no-pull")
	devCmd.Flags().BoolVar(&localReset, "local-reset", false, "Remove local package (use after ast dev --local); run 'bun install' to restore deps")
	_ = devCmd.Flags().MarkHidden("local")
	_ = devCmd.Flags().MarkHidden("local-reset")
}

func runDev(cmd *cobra.Command, args []string) error {
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
		log.Printf("📦 Removed local packages. Run 'bun install' to restore dependencies.")
		return nil
	}

	// --local implies --no-pull and requires ASTRO_ROOT for local packages
	if local {
		noPull = true
		if os.Getenv("ASTRO_ROOT") == "" {
			return fmt.Errorf("ASTRO_ROOT is not set (required for --local to use local packages)")
		}
	}

	log.Printf("🚀 Starting Astro dev mode...")
	log.Printf("📄 Loading spec from: %s", specFile)

	// Parse astro.yml
	specPath := filepath.Join(workingDir, specFile)
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	log.Printf("✅ Loaded spec for agent: %s", astroSpec.Name)

	// Load .env file
	envVars, err := utils.LoadEnvFile(workingDir, envFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}
	if envVars == nil {
		envVars = make(map[string]string)
		log.Printf("⚠️  No .env file found at %s (continuing without integration credentials)", envFile)
	} else {
		log.Printf("🔑 Loading environment from: %s", envFile)
		var envKeys []string
		for key := range envVars {
			envKeys = append(envKeys, key)
		}
		log.Printf("   Loaded %d environment variables: %s", len(envKeys), strings.Join(envKeys, ", "))
		for key, val := range envVars {
			os.Setenv(key, val)
		}
	}
	if t := getGitHubPackagesToken(); t != "" {
		envVars["GITHUB_PACKAGES_TOKEN"] = t
		os.Setenv("GITHUB_PACKAGES_TOKEN", t)
	}

	// Build Docker Compose project
	log.Printf("🐳 Building Docker Compose project...")
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
			log.Printf("   --local: using local image names (no pull)")
		}
	}

	// --local: omit agent from compose and run it as a local process
	if local {
		delete(project.Services, "agent")
		if verbose {
			log.Printf("   --local: agent will run as local process")
		}
	}

	if verbose {
		log.Printf("   Services: %d", len(project.Services))
		for name := range project.Services {
			log.Printf("     - %s", name)
		}
	}

	// Write docker-compose.yml file
	composePath := filepath.Join(workingDir, ".astro", "docker-compose.yml")
	if err := os.MkdirAll(filepath.Dir(composePath), 0755); err != nil {
		return fmt.Errorf("failed to create .astro directory: %w", err)
	}

	composeData, err := yaml.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal compose project: %w", err)
	}

	if err := os.WriteFile(composePath, composeData, 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	if verbose {
		log.Printf("   Wrote compose file to: %s", composePath)
	}

	// Check for GITHUB_PACKAGES_TOKEN before building (unless skipping pull)
	if !noPull {
		ghcrToken := getGitHubPackagesToken()
		if ghcrToken == "" {
			return fmt.Errorf("GITHUB_PACKAGES_TOKEN is required (set in env or use a CLI build that injects it)")
		}

		// Login to GHCR (for pulling astro-messaging image)
		log.Printf("🔑 Logging into GHCR...")
		loginCmd := exec.Command("docker", "login", "ghcr.io", "-u", "saswatds", "--password-stdin")
		loginCmd.Stdin = strings.NewReader(ghcrToken)
		if err := loginCmd.Run(); err != nil {
			log.Printf("⚠️  GHCR login failed: %v (continuing anyway)", err)
		}
	}

	// Start services using Docker Compose CLI
	log.Printf("🔨 Building and starting services...")

	// Build with or without cache based on rebuild flag
	if rebuild {
		log.Printf("   Using --no-cache for clean rebuild...")
		buildCmd := exec.Command("docker", "compose", "-f", composePath, "build", "--no-cache")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build services: %w", err)
		}
	}

	upArgs := []string{"compose", "-f", composePath, "up", "-d", "--build"}
	if noPull {
		upArgs = append(upArgs, "--pull=never")
	}
	upCmd := exec.Command("docker", upArgs...)
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	log.Printf("✅ All services running!")

	// Run agent as local process when --local
	var (
		agentCmd       *exec.Cmd
		agentCmdMu     sync.Mutex
		agentRestartCh chan struct{}
		agentCtx       context.Context
		agentCancel    context.CancelFunc
	)
	if local {
		agentCtx, agentCancel = context.WithCancel(context.Background())
		agentRestartCh = make(chan struct{}, 1)
		agentEnv := buildLocalAgentEnv(astroSpec, envVars)
		workingDirForAgent := workingDir

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
		log.Printf("📦 Using local packages from %s", astroRoot)

		runAgent := func() {
			cmd := exec.CommandContext(agentCtx, "bun", "run", "start")
			cmd.Dir = workingDirForAgent
			cmd.Env = agentEnv
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			agentCmdMu.Lock()
			agentCmd = cmd
			agentCmdMu.Unlock()
			if err := cmd.Start(); err != nil {
				log.Printf("❌ Failed to start agent: %v (is bun installed? run from project root with package.json)", err)
				return
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case <-agentCtx.Done():
				_ = cmd.Process.Kill()
				<-done
			case <-agentRestartCh:
				_ = cmd.Process.Kill()
				<-done
			case <-done:
				// process exited (e.g. crash)
			}
		}

		go func() {
			for {
				runAgent()
				if agentCtx.Err() != nil {
					return
				}
				log.Printf("🔄 Restarting agent...")
			}
		}()

		// Give the agent a moment to start so we don't spam restart on first run
		time.Sleep(500 * time.Millisecond)
		log.Printf("🤖 Agent running as local process (bun run start)")
	}

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

	if hasMessagingInterface {
		log.Printf("💬 Messaging service running on gRPC port 9090")
	}

	if hasWebInterface {
		log.Printf("🌐 Playground running at http://localhost:3000")
		log.Printf("   Web API available at http://localhost:3100")

		// Open playground in browser
		go func() {
			// Small delay to ensure services are ready
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

				log.Printf("⏰ Scheduling ingestion '%s' with pattern: %s", ingestionName, cronPattern)

				// Add cron job
				_, err := cronScheduler.AddFunc(cronPattern, func() {
					log.Printf("🔄 Running ingestion: %s", ingestionName)

					// Run the ingestion container
					serviceName := fmt.Sprintf("ingestion-%s", ingestionName)
					runCmd := exec.Command("docker", "compose", "-f", composePath, "run", "--rm", serviceName)
					runCmd.Stdout = os.Stdout
					runCmd.Stderr = os.Stderr

					if err := runCmd.Run(); err != nil {
						log.Printf("❌ Failed to run ingestion '%s': %v", ingestionName, err)
					} else {
						log.Printf("✅ Ingestion '%s' completed", ingestionName)
					}
				})

				if err != nil {
					log.Printf("⚠️  Failed to schedule ingestion '%s': %v", ingestionName, err)
				}
			} else if ingestion.Trigger.Type == "startup" {
				// Run immediately on startup
				ingestionName := name
				log.Printf("🚀 Running startup ingestion: %s", ingestionName)

				serviceName := fmt.Sprintf("ingestion-%s", ingestionName)
				go func() {
					runCmd := exec.Command("docker", "compose", "-f", composePath, "run", "--rm", serviceName)
					runCmd.Stdout = os.Stdout
					runCmd.Stderr = os.Stderr
					if err := runCmd.Run(); err != nil {
						log.Printf("❌ Failed to run startup ingestion '%s': %v", ingestionName, err)
					} else {
						log.Printf("✅ Startup ingestion '%s' completed", ingestionName)
					}
				}()
			}
		}

		if cronScheduler != nil {
			cronScheduler.Start()
			defer cronScheduler.Stop()
			log.Printf("📅 Ingestion scheduler started")
		}
	}

	// Set up file watcher for hot reload
	if !noReload {
		log.Printf("👀 Watching for file changes in ./agent...")

		agentDir := filepath.Join(workingDir, "agent")
		if _, err := os.Stat(agentDir); err == nil {
			fw, err := watcher.New(agentDir, func(path string) {
				log.Printf("📝 File changed: %s", path)
				if local && agentRestartCh != nil {
					select {
					case agentRestartCh <- struct{}{}:
					default:
					}
					log.Printf("✅ Agent reloaded!")
					return
				}
				log.Printf("🔄 Rebuilding agent...")

				// Rebuild and restart only the agent service
				buildCmd := exec.Command("docker", "compose", "-f", composePath, "build", "agent")
				buildCmd.Stdout = os.Stdout
				buildCmd.Stderr = os.Stderr
				if err := buildCmd.Run(); err != nil {
					log.Printf("❌ Build failed: %v", err)
					return
				}

				log.Printf("♻️  Restarting agent...")
				restartCmd := exec.Command("docker", "compose", "-f", composePath, "restart", "agent")
				restartCmd.Stdout = os.Stdout
				restartCmd.Stderr = os.Stderr
				if err := restartCmd.Run(); err != nil {
					log.Printf("❌ Restart failed: %v", err)
					return
				}

				log.Printf("✅ Agent reloaded!")
			})

			if err != nil {
				log.Printf("⚠️  Failed to create file watcher: %v (hot reload disabled)", err)
			} else {
				if err := fw.Start(); err != nil {
					log.Printf("⚠️  Failed to start file watcher: %v (hot reload disabled)", err)
				}
				defer fw.Stop()
			}
		} else {
			log.Printf("⚠️  ./agent directory not found (hot reload disabled)")
		}
	}

	// Tail agent container logs (only in container mode, not --local)
	var logsCancel context.CancelFunc
	if !local {
		var logsCtx context.Context
		logsCtx, logsCancel = context.WithCancel(context.Background())
		logsCmd := exec.CommandContext(logsCtx, "docker", "compose", "-f", composePath, "logs", "-f", "agent")
		logsCmd.Stdout = os.Stdout
		logsCmd.Stderr = os.Stderr
		go func() {
			if err := logsCmd.Run(); err != nil && logsCtx.Err() == nil {
				log.Printf("⚠️  Agent logs stream ended: %v", err)
			}
		}()
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("")
	log.Printf("✨ Ready! Press Ctrl+C to stop")
	log.Printf("")

	<-sigChan

	if logsCancel != nil {
		logsCancel()
	}

	log.Printf("")
	log.Printf("🛑 Shutting down...")

	if local && agentCancel != nil {
		agentCancel()
		agentCmdMu.Lock()
		if agentCmd != nil && agentCmd.Process != nil {
			_ = agentCmd.Process.Kill()
		}
		agentCmdMu.Unlock()
	}

	// Stop all services
	downCmd := exec.Command("docker", "compose", "-f", composePath, "down")
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	if err := downCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	log.Printf("✅ Cleanup complete")
	if local {
		log.Printf("💡 Tip: run 'ast dev --local-reset' to remove injected local dependencies")
	}

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

// localAstroPackages are the @saswatds/* packages we link in --local and remove in --local-reset.
var localAstroPackages = []string{
	"astro-agent", "astro-graph", "astro-messaging",
}

// linkLocalPackages symlinks node_modules/@saswatds/* to the given Astro repo packages/
// so the agent uses local source in --local mode.
func linkLocalPackages(workingDir, astroRoot string) error {
	scopeDir := filepath.Join(workingDir, "node_modules", "@saswatds")
	if err := os.MkdirAll(scopeDir, 0755); err != nil {
		return err
	}
	for _, pkg := range localAstroPackages {
		target := filepath.Join(astroRoot, "packages", pkg)
		target, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if st, err := os.Stat(target); err != nil {
			return fmt.Errorf("%s: %w", pkg, err)
		} else if !st.IsDir() {
			return fmt.Errorf("%s is not a directory", target)
		}
		link := filepath.Join(scopeDir, pkg)
		_ = os.RemoveAll(link)
		if err := os.Symlink(target, link); err != nil {
			return fmt.Errorf("symlink %s: %w", pkg, err)
		}
	}
	return nil
}

// unlinkLocalPackages removes the @saswatds/* symlinks (or dirs) created by linkLocalPackages
// so the user can run bun install to restore registry dependencies.
func unlinkLocalPackages(workingDir string) error {
	scopeDir := filepath.Join(workingDir, "node_modules", "@saswatds")
	for _, pkg := range localAstroPackages {
		path := filepath.Join(scopeDir, pkg)
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", pkg, err)
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
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		log.Printf("⚠️  Unable to open browser automatically on %s", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("⚠️  Failed to open browser: %v", err)
	}
}
