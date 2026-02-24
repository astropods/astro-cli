package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/utils"
)

const playgroundImage = "astropods/playground:latest"

var playgroundCmd = &cobra.Command{
	Use:   "playground <url>",
	Short: "Open the Astro playground connected to a running agent",
	Long: `Start the Astro playground UI and point it at a running agent backend.

The URL should be the base URL of the astro-messaging HTTP API
(the endpoint that serves /health and /api/*).

Example:
  ast playground http://localhost:3100
  ast playground https://my-agent.example.com
  ast playground http://localhost:3100 --port 4000
  ast playground http://localhost:3100 --local
  ast playground http://localhost:3100 --no-pull`,
	Args: cobra.ExactArgs(1),
	RunE: runPlayground,
}

var (
	playgroundPort   string
	playgroundNoPull bool
	playgroundNoOpen bool
	playgroundLocal  bool
)

func init() {
	rootCmd.AddCommand(playgroundCmd)
	playgroundCmd.Flags().StringVar(&playgroundPort, "port", "3737", "Local port for the playground UI")
	playgroundCmd.Flags().BoolVar(&playgroundNoPull, "no-pull", false, "Skip pulling the playground image")
	playgroundCmd.Flags().BoolVar(&playgroundLocal, "local", false, "Use locally built playground image; do not pull (implies --no-pull)")
	playgroundCmd.Flags().BoolVar(&playgroundNoOpen, "no-open", false, "Don't open the browser automatically")
	_ = playgroundCmd.Flags().MarkHidden("local")
}

func runPlayground(cmd *cobra.Command, args []string) error {
	apiURL := strings.TrimRight(args[0], "/")

	if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}

	// Load .env file if present
	workingDir, _ := os.Getwd()
	if envMap, _ := utils.LoadEnvFile(workingDir, utils.DefaultEnvFile); envMap != nil {
		for key, val := range envMap {
			_ = os.Setenv(key, val)
		}
	}

	if playgroundLocal {
		playgroundNoPull = true
	}

	imageToUse := utils.ImageNameForLocal(playgroundImage, playgroundLocal)

	log.Printf("🎮 Starting Astro Playground...")
	log.Printf("   Backend: %s", apiURL)
	if playgroundLocal {
		log.Printf("   Using local image: %s", imageToUse)
	}

	// Pull latest image unless --no-pull or --local
	if !playgroundNoPull {
		log.Printf("📦 Pulling playground image...")
		pullCmd := exec.Command("docker", "pull", imageToUse) //nolint:gosec
		pullCmd.Stdout = os.Stdout
		pullCmd.Stderr = os.Stderr
		if err := pullCmd.Run(); err != nil {
			return fmt.Errorf("failed to pull playground image: %w", err)
		}
	}

	// Rewrite localhost URLs so the container can reach the host
	containerBackendURL := apiURL
	containerBackendURL = strings.Replace(containerBackendURL, "://localhost", "://host.docker.internal", 1)
	containerBackendURL = strings.Replace(containerBackendURL, "://127.0.0.1", "://host.docker.internal", 1)
	if containerBackendURL != apiURL {
		log.Printf("   Container backend: %s", containerBackendURL)
	}

	// Start the container
	containerName := "astro-playground"
	log.Printf("🐳 Starting playground container...")

	// Remove any stale container with the same name
	rmCmd := exec.Command("docker", "rm", "-f", containerName) //nolint:gosec
	_ = rmCmd.Run()                                            // ignore errors – container may not exist

	runArgs := []string{
		"run", "-d",
		"--name", containerName,
		"--add-host=host.docker.internal:host-gateway",
		"-p", playgroundPort + ":80",
		"-e", "BACKEND_URL=" + containerBackendURL,
		imageToUse,
	}
	dockerRun := exec.Command("docker", runArgs...) //nolint:gosec
	dockerRun.Stdout = os.Stdout
	dockerRun.Stderr = os.Stderr
	if err := dockerRun.Run(); err != nil {
		return fmt.Errorf("failed to start playground container: %w", err)
	}

	playgroundURL := fmt.Sprintf("http://localhost:%s", playgroundPort)
	log.Printf("🌐 Playground running at %s", playgroundURL)

	if !playgroundNoOpen {
		go func() {
			time.Sleep(2 * time.Second)
			openBrowser(playgroundURL)
		}()
	}

	// Wait for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("")
	log.Printf("✨ Ready! Press Ctrl+C to stop")
	log.Printf("")

	<-sigChan

	log.Printf("")
	log.Printf("🛑 Shutting down...")

	stopCmd := exec.Command("docker", "rm", "-f", containerName) //nolint:gosec
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	if err := stopCmd.Run(); err != nil {
		return fmt.Errorf("failed to stop playground container: %w", err)
	}

	log.Printf("✅ Cleanup complete")
	return nil
}
