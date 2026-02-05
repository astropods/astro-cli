package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	composeBuilder "github.com/postman/astro/apps/astro-cli/internal/compose"
	"github.com/postman/astro/apps/astro-cli/internal/watcher"
	"github.com/postman/astro/packages/astro-spec"
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
  astro dev
  astro dev --file custom-astro.yml
  astro dev --env .env.local`,
	RunE: runDev,
}

var (
	envFile   string
	noReload  bool
	rebuild   bool
)

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.Flags().StringVar(&envFile, "env", ".env", "Environment file for integration credentials")
	devCmd.Flags().BoolVar(&noReload, "no-reload", false, "Disable hot reload")
	devCmd.Flags().BoolVar(&rebuild, "rebuild", false, "Force rebuild all containers without cache")
}

func runDev(cmd *cobra.Command, args []string) error {
	// Get spec file path
	specFile, _ := cmd.Flags().GetString("file")
	verbose, _ := cmd.Flags().GetBool("verbose")

	log.Printf("🚀 Starting Astro dev mode...")
	log.Printf("📄 Loading spec from: %s", specFile)

	// Parse astro.yml
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath := filepath.Join(workingDir, specFile)
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	log.Printf("✅ Loaded spec for agent: %s (v%s)", astroSpec.Agent, astroSpec.Meta.Version)

	// Load .env file
	envPath := filepath.Join(workingDir, envFile)
	envVars := make(map[string]string)

	if _, err := os.Stat(envPath); err == nil {
		log.Printf("🔑 Loading environment from: %s", envFile)
		envMap, err := godotenv.Read(envPath)
		if err != nil {
			return fmt.Errorf("failed to read .env file: %w", err)
		}
		envVars = envMap

		// Log loaded environment variable names (not values, for security)
		var envKeys []string
		for key := range envVars {
			envKeys = append(envKeys, key)
		}
		log.Printf("   Loaded %d environment variables: %s", len(envKeys), strings.Join(envKeys, ", "))

		// Export env vars to OS environment for Docker build secrets
		for key, val := range envVars {
			os.Setenv(key, val)
		}
	} else {
		log.Printf("⚠️  No .env file found at %s (continuing without integration credentials)", envFile)
	}

	// Build Docker Compose project
	log.Printf("🐳 Building Docker Compose project...")
	project, err := composeBuilder.BuildProject(astroSpec, workingDir, envVars)
	if err != nil {
		return fmt.Errorf("failed to build compose project: %w", err)
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

	// Login to GHCR if token is available (for pulling astro-messaging image)
	if ghcrToken := os.Getenv("GITHUB_PACKAGES_TOKEN"); ghcrToken != "" {
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

	upCmd := exec.Command("docker", "compose", "-f", composePath, "up", "-d", "--build")
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	log.Printf("✅ All services running!")

	// Check if messaging interface is configured
	hasMessagingInterface := false
	hasWebInterface := false
	for _, iface := range astroSpec.Interfaces {
		if iface.Type == "messaging/slack" || iface.Type == "messaging/discord" || iface.Type == "messaging/teams" {
			hasMessagingInterface = true
		}
		if iface.Type == "slack" || iface.Type == "web" {
			hasMessagingInterface = true
		}
		if iface.Type == "web" {
			hasWebInterface = true
		}
	}

	if hasMessagingInterface {
		log.Printf("💬 Messaging service running on gRPC port 9090")
	}

	if hasWebInterface {
		log.Printf("🌐 Playground running at http://localhost:3000")
		log.Printf("   Web API available at http://localhost:8080")

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
			if ingestion.Trigger.Type == "schedule" && ingestion.Trigger.Schedule != "" {
				cronPattern := ingestion.Trigger.Schedule
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

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("")
	log.Printf("✨ Ready! Press Ctrl+C to stop")
	log.Printf("")

	<-sigChan

	log.Printf("")
	log.Printf("🛑 Shutting down...")

	// Stop all services
	downCmd := exec.Command("docker", "compose", "-f", composePath, "down")
	downCmd.Stdout = os.Stdout
	downCmd.Stderr = os.Stderr
	if err := downCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop services: %w", err)
	}

	log.Printf("✅ Cleanup complete")

	return nil
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

