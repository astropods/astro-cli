package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
	"github.com/postman/astro/packages/astro-spec"
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish agent images and spec to astro platform",
	Long: `Publish container images to astro-registry and spec to astro-server.

This will:
1. Tag and push the agent container image to astro-registry
2. Tag and push custom-built component images (models, knowledge, tools)
3. Register the agent spec with astro-server

Images are pushed through the astro-registry service which proxies to ECR.
The spec is registered with astro-server which validates and stores it.

Example:
  astro publish
  astro publish --tag v1.0.0
  astro publish --build --tag v1.0.0

Requirements:
  - Must be authenticated with astro (run 'astro login' first)
  - Server and registry URLs must be configured (run 'astro configure')`,
	RunE: runPublish,
}

var (
	publishTag   string
	buildFirst   bool
	serverURL    string
	registryURL  string
	skipRegister bool
	noAuth       bool
	dryRun       bool
)

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().StringVarP(&publishTag, "tag", "t", "latest", "Tag to publish")
	publishCmd.Flags().BoolVar(&buildFirst, "build", false, "Build before publishing")
	publishCmd.Flags().StringVar(&serverURL, "server", "", "Astro server URL (overrides ASTRO_SERVER_URL)")
	publishCmd.Flags().StringVar(&registryURL, "registry", "", "Astro registry URL (overrides ASTRO_REGISTRY_URL)")
	publishCmd.Flags().BoolVar(&skipRegister, "skip-register", false, "Skip registering agent spec with server")
	publishCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Skip authentication (not recommended)")
	publishCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be published without actually doing it")
}

func runPublish(cmd *cobra.Command, args []string) error {
	// Get spec file path
	specFile, _ := cmd.Flags().GetString("file")
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Get server URL
	effectiveServerURL := serverURL
	if effectiveServerURL == "" {
		effectiveServerURL = auth.GetServerURL()
	}

	// Get registry URL
	effectiveRegistryURL := registryURL
	if effectiveRegistryURL == "" {
		effectiveRegistryURL = auth.GetRegistryURL()
	}

	if effectiveRegistryURL == "" {
		return fmt.Errorf("registry URL required: run 'ast configure', set ASTRO_REGISTRY_URL environment variable, or use --registry flag")
	}

	if effectiveServerURL == "" && !skipRegister && !dryRun {
		return fmt.Errorf("server URL required for registration: run 'ast configure', set ASTRO_SERVER_URL environment variable, use --server flag, or use --skip-register")
	}

	// Parse astro.yml
	fmt.Printf("%s→%s Parsing %s\n", colorCyan, colorReset, specFile)

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath := filepath.Join(workingDir, specFile)
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	// Build registry host from URL
	registryHost, err := getRegistryHost(effectiveRegistryURL)
	if err != nil {
		return fmt.Errorf("failed to parse registry URL: %w", err)
	}

	// Dry run mode - show what would be published and exit
	if dryRun {
		// Try to get namespace if authenticated, fall back to placeholder
		namespace := "<namespace>"
		if ns, err := getUserNamespace(effectiveRegistryURL, noAuth, verbose); err == nil {
			namespace = ns
		}
		return printDryRun(astroSpec, registryHost, namespace, publishTag, effectiveServerURL, skipRegister, buildFirst)
	}

	// Check authentication (only for actual publish)
	if !noAuth {
		tokenManager := auth.NewTokenManager()
		if !tokenManager.IsAuthenticated() {
			return fmt.Errorf("not authenticated. Run 'astro login' to authenticate")
		}
	}

	// Print header
	fmt.Printf("%s→%s Publishing %s%s%s v%s\n\n", colorCyan, colorReset, colorBold, astroSpec.Agent, colorReset, astroSpec.Meta.Version)

	// Step 1: Get namespace
	printStep("Authenticating with registry...")
	namespace, err := getUserNamespace(effectiveRegistryURL, noAuth, verbose)
	if err != nil {
		printStepFail()
		return fmt.Errorf("failed to get user namespace: %w", err)
	}
	printStepDone(fmt.Sprintf("namespace: %s", namespace))

	// Build images first if requested
	imagesPushed := 0

	if buildFirst {
		printStep("Building images")
		fmt.Println()
		if err := runBuild(cmd, args); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}
	}

	// Push images
	// 1. Publish agent container image
	localImageName := fmt.Sprintf("%s:%s", astroSpec.Agent, publishTag)
	remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, astroSpec.Agent, publishTag)

	printPushStart("agent", astroSpec.Agent)
	if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
		printPushComplete(false, 0)
		return fmt.Errorf("failed to tag agent image: %w", err)
	}
	size, err := pushImageToRegistry(localImageName, remoteImageName, noAuth, verbose)
	if err != nil {
		printPushComplete(false, 0)
		return fmt.Errorf("failed to push agent image: %w", err)
	}
	printPushComplete(true, size)
	imagesPushed++

	// 2. Publish custom-built model images
	for modelName, model := range astroSpec.Models {
		if model.Container.Build != nil {
			localImageName := fmt.Sprintf("%s-model-%s:%s", astroSpec.Agent, modelName, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s/%s-model-%s:%s", registryHost, namespace, astroSpec.Agent, modelName, publishTag)

			printPushStart("model", modelName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to tag model %s: %w", modelName, err)
			}
			size, err := pushImageToRegistry(localImageName, remoteImageName, noAuth, verbose)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push model %s: %w", modelName, err)
			}
			printPushComplete(true, size)
			imagesPushed++
		}
	}

	// 3. Publish custom-built knowledge store images
	for knowledgeName, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Build != nil {
			localImageName := fmt.Sprintf("%s-knowledge-%s:%s", astroSpec.Agent, knowledgeName, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s/%s-knowledge-%s:%s", registryHost, namespace, astroSpec.Agent, knowledgeName, publishTag)

			printPushStart("knowledge", knowledgeName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to tag knowledge store %s: %w", knowledgeName, err)
			}
			size, err := pushImageToRegistry(localImageName, remoteImageName, noAuth, verbose)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push knowledge store %s: %w", knowledgeName, err)
			}
			printPushComplete(true, size)
			imagesPushed++
		}
	}

	// 4. Publish custom-built tool images
	for toolName, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			localImageName := fmt.Sprintf("%s-tool-%s:%s", astroSpec.Agent, toolName, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s/%s-tool-%s:%s", registryHost, namespace, astroSpec.Agent, toolName, publishTag)

			printPushStart("tool", toolName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to tag tool %s: %w", toolName, err)
			}
			size, err := pushImageToRegistry(localImageName, remoteImageName, noAuth, verbose)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push tool %s: %w", toolName, err)
			}
			printPushComplete(true, size)
			imagesPushed++
		}
	}

	// 5. Publish custom-built interface service images
	for ifaceName, iface := range astroSpec.Interfaces {
		if iface.Service != nil && iface.Service.Build != nil {
			localImageName := fmt.Sprintf("%s-interface-%s:%s", astroSpec.Agent, ifaceName, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s/%s-interface-%s:%s", registryHost, namespace, astroSpec.Agent, ifaceName, publishTag)

			printPushStart("interface", ifaceName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to tag interface %s: %w", ifaceName, err)
			}
			size, err := pushImageToRegistry(localImageName, remoteImageName, noAuth, verbose)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push interface %s: %w", ifaceName, err)
			}
			printPushComplete(true, size)
			imagesPushed++
		}
	}

	fmt.Println()

	// Register agent spec with server
	if !skipRegister && effectiveServerURL != "" {
		printStep("Registering agent with server...")

		// Build the full registry path for the transformed spec
		registryPath := fmt.Sprintf("%s/%s", registryHost, namespace)
		if err := registerAgent(effectiveServerURL, astroSpec.Agent, astroSpec.Meta.Version, registryPath, specPath, publishTag, verbose, noAuth); err != nil {
			printStepFail()
			fmt.Printf("  %s%sWarning: Agent images were published, but registration failed%s\n", colorYellow, colorDim, colorReset)
			fmt.Printf("  %s%s%v%s\n", colorDim, colorReset, err, colorReset)
		} else {
			printStepDone("")
		}
	} else if skipRegister {
		printStep("Registering agent with server")
		fmt.Printf(" %s(skipped)%s\n", colorDim, colorReset)
	}

	// Final summary
	fmt.Printf("\n%s%s✓ Published successfully!%s\n", colorBold, colorGreen, colorReset)
	fmt.Printf("  %s%d%s image(s) pushed to %s%s%s\n\n", colorCyan, imagesPushed, colorReset, colorDim, registryHost, colorReset)

	return nil
}

// Progress output helpers
func printStep(message string) {
	fmt.Printf("%s→%s %s", colorCyan, colorReset, message)
}

func printStepDone(detail string) {
	if detail != "" {
		fmt.Printf(" %s✓%s %s%s%s\n", colorGreen, colorReset, colorDim, detail, colorReset)
	} else {
		fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
	}
}

func printStepFail() {
	fmt.Printf(" %s✗%s\n", colorRed, colorReset)
}

func printPushStart(componentType, name string) {
	fmt.Printf("%s→%s %s%s%s [%s]\n", colorCyan, colorReset, colorBold, name, colorReset, componentType)
}

func printPushComplete(success bool, _ int64) {
	if success {
		fmt.Printf("  %s✓%s done\n", colorGreen, colorReset)
	} else {
		fmt.Printf("  %s✗%s failed\n", colorRed, colorReset)
	}
}

// getUserNamespace fetches the user's namespace (user ID) from the registry service.
func getUserNamespace(registryURL string, skipAuth bool, verbose bool) (string, error) {
	reqURL := fmt.Sprintf("%s/api/namespace", strings.TrimSuffix(registryURL, "/"))
	req, err := http.NewRequestWithContext(context.Background(), "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	if !skipAuth {
		if verbose {
			log.Printf("   Adding auth header...")
		}
		if err := auth.AddAuthHeader(context.Background(), req); err != nil {
			return "", fmt.Errorf("failed to add authentication: %w", err)
		}
		if verbose {
			log.Printf("   Auth header added successfully")
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Read response body for more details
		body, _ := io.ReadAll(resp.Body)
		if verbose {
			log.Printf("   Registry auth failed: %s", string(body))
		}
		return "", fmt.Errorf("authentication required. Run 'astro login' to authenticate")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("registry returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		UserID         string `json:"user_id"`
		OrganizationID string `json:"organization_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.UserID == "" {
		return "", fmt.Errorf("registry returned empty user ID")
	}

	if verbose {
		log.Printf("   User ID: %s", result.UserID)
		if result.OrganizationID != "" {
			log.Printf("   Organization ID: %s", result.OrganizationID)
		}
	}

	// Docker/OCI registry names must be lowercase
	return strings.ToLower(result.UserID), nil
}

// getRegistryHost extracts the host from the registry URL for use as registry address.
func getRegistryHost(registryURL string) (string, error) {
	u, err := url.Parse(registryURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func tagImage(sourceImage, targetImage string, verbose bool) error {
	tagCmd := exec.Command("docker", "tag", sourceImage, targetImage)

	if verbose {
		tagCmd.Stdout = os.Stdout
		tagCmd.Stderr = os.Stderr
	}

	if err := tagCmd.Run(); err != nil {
		return fmt.Errorf("failed to tag image: %w", err)
	}

	return nil
}

// pushImageToRegistry pushes an image to the astro-registry service with progress tracking.
// Returns the size of the pushed image in bytes.
func pushImageToRegistry(localImageName, remoteImageName string, skipAuth bool, _ bool) (int64, error) {
	fmt.Printf("  %sloading...%s", colorDim, colorReset)

	// Parse the local image name reference
	localRef, err := name.ParseReference(localImageName)
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("failed to parse local image name: %w", err)
	}

	// Load the image from Docker daemon
	img, err := daemon.Image(localRef)
	if err != nil {
		fmt.Println()
		return 0, fmt.Errorf("failed to load image from Docker daemon: %w", err)
	}

	// Get image size for progress bar
	var imageSize int64
	layers, err := img.Layers()
	if err == nil {
		for _, layer := range layers {
			size, _ := layer.Size()
			imageSize += size
		}
	}

	// Clear "loading..." and move to next line for progress bar
	fmt.Print("\r                    \r")

	// Parse the remote image name reference
	remoteRef, err := name.ParseReference(remoteImageName)
	if err != nil {
		return 0, fmt.Errorf("failed to parse remote image name: %w", err)
	}

	// Build remote options with authentication
	var opts []remote.Option
	if !skipAuth {
		opts = append(opts, remote.WithAuth(auth.GetCraneAuth()))
	}

	// Set up progress tracking
	progressChan := make(chan v1.Update, 100)
	opts = append(opts, remote.WithProgress(progressChan))

	// Create progress bar
	bar := progressbar.NewOptions64(
		imageSize,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(25),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[cyan]█[reset]",
			SaucerHead:    "[cyan]█[reset]",
			SaucerPadding: "░",
			BarStart:      "",
			BarEnd:        "",
		}),
		progressbar.OptionSetRenderBlankState(true),
	)

	// Update progress bar from channel
	done := make(chan struct{})
	go func() {
		defer close(done)
		for update := range progressChan {
			if update.Complete > 0 {
				bar.Set64(update.Complete)
			}
		}
		bar.Finish()
	}()

	// Push the image to the registry
	pushErr := remote.Write(remoteRef, img, opts...)

	// Wait for progress display to finish
	<-done

	if pushErr != nil {
		return 0, fmt.Errorf("failed to push image: %w", pushErr)
	}

	return imageSize, nil
}

// registerAgent registers the agent spec with the astro-server
func registerAgent(serverURL, agentName, version, registry, specPath, publishTag string, verbose bool, skipAuth bool) error {
	// Read and parse spec file
	specData, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	// Parse YAML spec
	var specObj map[string]interface{}
	if err := yaml.Unmarshal(specData, &specObj); err != nil {
		return fmt.Errorf("failed to parse spec YAML: %w", err)
	}

	// Transform spec: replace build sections with actual image references
	specObj = transformSpecForRegistry(specObj, registry, agentName, publishTag)

	// Marshal back to YAML
	transformedSpecData, err := yaml.Marshal(specObj)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed spec: %w", err)
	}

	// Prepare request payload
	payload := map[string]string{
		"name":         agentName,
		"version":      version,
		"registry":     registry,
		"spec_content": string(transformedSpecData),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send POST request to server
	reqURL := fmt.Sprintf("%s/api/v1/agents/register", strings.TrimSuffix(serverURL, "/"))
	req, err := http.NewRequestWithContext(context.Background(), "POST", reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication header if not skipped
	if !skipAuth {
		if err := auth.AddAuthHeader(context.Background(), req); err != nil {
			// Check if we should fail or just warn
			tokenManager := auth.NewTokenManager()
			if tokenManager.IsAuthenticated() {
				return fmt.Errorf("failed to add authentication: %w", err)
			}
			log.Printf("Warning: Not authenticated. Use 'astro login' for authenticated requests or --no-auth to skip")
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication required. Run 'astro login' to authenticate")
	}

	if resp.StatusCode != http.StatusCreated {
		// Read the full response body for detailed error logging
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("server returned status %d (failed to read response body: %v)", resp.StatusCode, readErr)
		}

		// Log the raw error response
		log.Printf("Registration failed with status %d. Response body: %s", resp.StatusCode, string(body))

		// Try to parse as JSON for structured error
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return fmt.Errorf("server returned error (status %d): %v", resp.StatusCode, errorResp)
		}

		// If not JSON, return the raw body
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	if verbose {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			log.Printf("   Server response: %v", result)
		}
	}

	return nil
}

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

// printDryRun explains the spec and shows what would be published
func printDryRun(astroSpec *spec.AstroSpec, registryHost, namespace, tag, serverURL string, skipRegister, buildFirst bool) error {
	fmt.Printf("\n%s%s=== Spec Explanation ===%s\n\n", colorBold, colorCyan, colorReset)

	// Agent overview
	fmt.Printf("%s%sAgent:%s %s\n", colorBold, colorGreen, colorReset, astroSpec.Agent)
	fmt.Printf("This agent is at version %s%s%s", colorYellow, astroSpec.Meta.Version, colorReset)
	if astroSpec.Meta.Owner != "" {
		fmt.Printf(" and is owned by %s%s%s", colorCyan, astroSpec.Meta.Owner, colorReset)
	}
	fmt.Println(".")
	if astroSpec.Meta.Description != "" {
		fmt.Printf("%s%s%s\n", colorDim, astroSpec.Meta.Description, colorReset)
	}
	if len(astroSpec.Meta.Tags) > 0 {
		fmt.Printf("Tagged as: %s%v%s\n", colorDim, astroSpec.Meta.Tags, colorReset)
	}
	fmt.Println()

	// Container
	fmt.Printf("%s%sContainer%s\n", colorBold, colorBlue, colorReset)
	if astroSpec.Container.Build != nil {
		fmt.Printf("The main agent runtime will be built from %s%s%s", colorYellow, astroSpec.Container.Build.Context, colorReset)
		if astroSpec.Container.Build.Dockerfile != "" {
			fmt.Printf(" using %s%s%s", colorYellow, astroSpec.Container.Build.Dockerfile, colorReset)
		}
		fmt.Println(".")
		fmt.Printf("It will be pushed to %s%s/%s/%s:%s%s\n", colorGreen, registryHost, namespace, astroSpec.Agent, tag, colorReset)
	} else if astroSpec.Container.Image != "" {
		fmt.Printf("The agent uses a pre-built image %s%s%s, so no push is needed.\n", colorYellow, astroSpec.Container.Image, colorReset)
	}
	fmt.Println()

	// Models
	if len(astroSpec.Models) > 0 {
		fmt.Printf("%s%sModels%s\n", colorBold, colorBlue, colorReset)
		fmt.Println("These are AI/ML model services that the agent can use for inference.")
		for name, model := range astroSpec.Models {
			fmt.Printf("\n  %s%s%s uses the %s%s%s provider", colorCyan, name, colorReset, colorYellow, model.Provider, colorReset)
			if model.Model != "" {
				fmt.Printf(" with model %s%s%s", colorYellow, model.Model, colorReset)
			}
			fmt.Println(".")
			if model.Container.Build != nil {
				fmt.Printf("  This model service will be built from %s%s%s and pushed to the registry.\n", colorYellow, model.Container.Build.Context, colorReset)
			} else if model.Container.Image != "" {
				fmt.Printf("  It uses a pre-built image %s%s%s.\n", colorDim, model.Container.Image, colorReset)
			}
		}
		fmt.Println()
	}

	// Knowledge
	if len(astroSpec.Knowledge) > 0 {
		fmt.Printf("%s%sKnowledge%s\n", colorBold, colorBlue, colorReset)
		fmt.Println("These are data stores that provide memory and context to the agent.")
		for name, k := range astroSpec.Knowledge {
			fmt.Printf("\n  %s%s%s is a %s%s%s store", colorCyan, name, colorReset, colorYellow, k.Type, colorReset)
			if k.Provider != "" {
				fmt.Printf(" powered by %s%s%s", colorYellow, k.Provider, colorReset)
			}
			fmt.Println(".")
			if k.Embedding != "" {
				fmt.Printf("  It uses %s%s%s for generating embeddings.\n", colorYellow, k.Embedding, colorReset)
			}
			if k.Container.Persistent {
				fmt.Printf("  Data is %s%spersistent%s and will survive container restarts.\n", colorBold, colorGreen, colorReset)
			}
			if k.Container.Build != nil {
				fmt.Printf("  This store will be built from %s%s%s and pushed to the registry.\n", colorYellow, k.Container.Build.Context, colorReset)
			} else if k.Container.Image != "" {
				fmt.Printf("  It uses a pre-built image %s%s%s.\n", colorDim, k.Container.Image, colorReset)
			}
		}
		fmt.Println()
	}

	// Tools
	if len(astroSpec.Tools) > 0 {
		fmt.Printf("%s%sTools%s\n", colorBold, colorBlue, colorReset)
		fmt.Println("These are capabilities that extend what the agent can do.")
		for name, tool := range astroSpec.Tools {
			fmt.Printf("\n  %s%s%s is a %s%s%s tool", colorCyan, name, colorReset, colorYellow, tool.Type, colorReset)
			if tool.Container != nil {
				if tool.Container.Build != nil {
					fmt.Println(" that runs in its own container.")
					fmt.Printf("  It will be built from %s%s%s and pushed to the registry.\n", colorYellow, tool.Container.Build.Context, colorReset)
				} else if tool.Container.Image != "" {
					fmt.Println(" that runs in its own container.")
					fmt.Printf("  It uses a pre-built image %s%s%s.\n", colorDim, tool.Container.Image, colorReset)
				}
			} else {
				fmt.Println(" that runs inside the main agent container.")
			}
		}
		fmt.Println()
	}

	// Integrations
	if len(astroSpec.Integrations.Models) > 0 || len(astroSpec.Integrations.Tools) > 0 || len(astroSpec.Integrations.Knowledge) > 0 {
		fmt.Printf("%s%sIntegrations%s\n", colorBold, colorBlue, colorReset)
		fmt.Println("These are external services that the agent connects to. No containers are needed.")
		for _, m := range astroSpec.Integrations.Models {
			fmt.Printf("\n  %s%s%s connects to %s%s%s", colorCyan, m.Name, colorReset, colorYellow, m.Provider, colorReset)
			if m.Model != "" {
				fmt.Printf(" using the %s%s%s model", colorYellow, m.Model, colorReset)
			}
			fmt.Println(" for inference.")
		}
		for _, t := range astroSpec.Integrations.Tools {
			fmt.Printf("\n  %s%s%s integrates with %s%s%s for external tooling.\n", colorCyan, t.Name, colorReset, colorYellow, t.Provider, colorReset)
		}
		for _, k := range astroSpec.Integrations.Knowledge {
			fmt.Printf("\n  %s%s%s connects to %s%s%s for external data.\n", colorCyan, k.Name, colorReset, colorYellow, k.Provider, colorReset)
		}
		fmt.Println()
	}

	// Interfaces
	if len(astroSpec.Interfaces) > 0 {
		fmt.Printf("%s%sInterfaces%s\n", colorBold, colorBlue, colorReset)
		fmt.Println("These define how users and systems interact with the agent.")
		for name, iface := range astroSpec.Interfaces {
			fmt.Printf("\n  %s%s%s provides a %s%s%s interface", colorCyan, name, colorReset, colorYellow, iface.Type, colorReset)
			if iface.Service != nil {
				if iface.Service.Build != nil {
					fmt.Println(" with a custom service.")
					fmt.Printf("  It will be built from %s%s%s and pushed to the registry.\n", colorYellow, iface.Service.Build.Context, colorReset)
				} else if iface.Service.Image != "" {
					fmt.Println(" with a custom service.")
					fmt.Printf("  It uses a pre-built image %s%s%s.\n", colorDim, iface.Service.Image, colorReset)
				}
			} else {
				fmt.Println(".")
			}
		}
		fmt.Println()
	}

	// Ingestion
	if len(astroSpec.Ingestion) > 0 {
		fmt.Printf("%s%sIngestion%s\n", colorBold, colorBlue, colorReset)
		fmt.Println("These are background jobs that sync data into the agent's knowledge stores.")
		for name, ing := range astroSpec.Ingestion {
			fmt.Printf("\n  %s%s%s runs on a %s%s%s trigger", colorCyan, name, colorReset, colorYellow, ing.Trigger.Type, colorReset)
			if ing.Trigger.Schedule != "" {
				fmt.Printf(" with schedule %s%s%s", colorYellow, ing.Trigger.Schedule, colorReset)
			}
			fmt.Println(".")
			if ing.Container.Build != nil {
				fmt.Printf("  It will be built from %s%s%s and pushed to the registry.\n", colorYellow, ing.Container.Build.Context, colorReset)
			} else if ing.Container.Image != "" {
				fmt.Printf("  It uses a pre-built image %s%s%s.\n", colorDim, ing.Container.Image, colorReset)
			}
		}
		fmt.Println()
	}

	// Summary of actions
	fmt.Printf("%s%s=== Publish Actions ===%s\n\n", colorBold, colorCyan, colorReset)

	stepNum := 1
	if buildFirst {
		fmt.Printf("%s%sStep %d: Build Images%s\n", colorBold, colorMagenta, stepNum, colorReset)
		fmt.Println("The following images will be built locally:")
		fmt.Printf("  • %s%s:%s%s\n", colorYellow, astroSpec.Agent, tag, colorReset)
		for name, model := range astroSpec.Models {
			if model.Container.Build != nil {
				fmt.Printf("  • %s%s-model-%s:%s%s\n", colorYellow, astroSpec.Agent, name, tag, colorReset)
			}
		}
		for name, k := range astroSpec.Knowledge {
			if k.Container.Build != nil {
				fmt.Printf("  • %s%s-knowledge-%s:%s%s\n", colorYellow, astroSpec.Agent, name, tag, colorReset)
			}
		}
		for name, tool := range astroSpec.Tools {
			if tool.Container != nil && tool.Container.Build != nil {
				fmt.Printf("  • %s%s-tool-%s:%s%s\n", colorYellow, astroSpec.Agent, name, tag, colorReset)
			}
		}
		for name, iface := range astroSpec.Interfaces {
			if iface.Service != nil && iface.Service.Build != nil {
				fmt.Printf("  • %s%s-interface-%s:%s%s\n", colorYellow, astroSpec.Agent, name, tag, colorReset)
			}
		}
		for name, ing := range astroSpec.Ingestion {
			if ing.Container.Build != nil {
				fmt.Printf("  • %s%s-ingestion-%s:%s%s\n", colorYellow, astroSpec.Agent, name, tag, colorReset)
			}
		}
		fmt.Println()
		stepNum++
	}

	fmt.Printf("%s%sStep %d: Push to Registry%s\n", colorBold, colorMagenta, stepNum, colorReset)
	if !buildFirst {
		fmt.Printf("%sNote: Images must already exist locally.%s\n", colorDim, colorReset)
	}

	pushCount := 0
	var pushTargets []string
	if astroSpec.Container.Build != nil {
		target := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, astroSpec.Agent, tag)
		pushTargets = append(pushTargets, target)
		pushCount++
	}
	for name, model := range astroSpec.Models {
		if model.Container.Build != nil {
			target := fmt.Sprintf("%s/%s/%s-model-%s:%s", registryHost, namespace, astroSpec.Agent, name, tag)
			pushTargets = append(pushTargets, target)
			pushCount++
		}
	}
	for name, k := range astroSpec.Knowledge {
		if k.Container.Build != nil {
			target := fmt.Sprintf("%s/%s/%s-knowledge-%s:%s", registryHost, namespace, astroSpec.Agent, name, tag)
			pushTargets = append(pushTargets, target)
			pushCount++
		}
	}
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			target := fmt.Sprintf("%s/%s/%s-tool-%s:%s", registryHost, namespace, astroSpec.Agent, name, tag)
			pushTargets = append(pushTargets, target)
			pushCount++
		}
	}
	for name, iface := range astroSpec.Interfaces {
		if iface.Service != nil && iface.Service.Build != nil {
			target := fmt.Sprintf("%s/%s/%s-interface-%s:%s", registryHost, namespace, astroSpec.Agent, name, tag)
			pushTargets = append(pushTargets, target)
			pushCount++
		}
	}
	for name, ing := range astroSpec.Ingestion {
		if ing.Container.Build != nil {
			target := fmt.Sprintf("%s/%s/%s-ingestion-%s:%s", registryHost, namespace, astroSpec.Agent, name, tag)
			pushTargets = append(pushTargets, target)
			pushCount++
		}
	}

	if pushCount == 0 {
		fmt.Printf("%sNo custom images to push. All components use pre-built images.%s\n", colorDim, colorReset)
	} else {
		fmt.Println("The following images will be pushed to the registry:")
		for _, target := range pushTargets {
			fmt.Printf("  • %s%s%s\n", colorGreen, target, colorReset)
		}
	}
	fmt.Println()

	stepNum++
	fmt.Printf("%s%sStep %d: Register Agent%s\n", colorBold, colorMagenta, stepNum, colorReset)
	if skipRegister {
		fmt.Printf("%sSkipped because --skip-register flag was set.%s\n", colorDim, colorReset)
	} else if serverURL == "" {
		fmt.Printf("%sSkipped because no server URL is configured.%s\n", colorDim, colorReset)
	} else {
		fmt.Printf("The agent spec will be registered with the server at %s%s%s.\n", colorYellow, serverURL, colorReset)
		fmt.Printf("Build references will be transformed to point to the pushed images.\n")
	}

	fmt.Printf("\n%s%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorDim, colorCyan, colorReset)
	fmt.Printf("Total images to push: %s%d%s\n", colorBold, pushCount, colorReset)
	fmt.Printf("%s%sThis is a dry run. No changes were made.%s\n\n", colorBold, colorYellow, colorReset)
	return nil
}

// transformSpecForRegistry replaces build sections with actual image references
func transformSpecForRegistry(specObj map[string]interface{}, registry, agentName, tag string) map[string]interface{} {
	// Replace container.build with container.image
	if container, ok := specObj["container"].(map[string]interface{}); ok {
		if _, hasBuild := container["build"]; hasBuild {
			delete(container, "build")
			container["image"] = fmt.Sprintf("%s/%s:%s", registry, agentName, tag)
		}
	}

	// Replace models.*.container.build with models.*.container.image
	if models, ok := specObj["models"].(map[string]interface{}); ok {
		for modelName, modelData := range models {
			if model, ok := modelData.(map[string]interface{}); ok {
				if container, ok := model["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-model-%s:%s", registry, agentName, modelName, tag)
					}
				}
			}
		}
	}

	// Replace knowledge.*.container.build with knowledge.*.container.image
	if knowledge, ok := specObj["knowledge"].(map[string]interface{}); ok {
		for knowledgeName, knowledgeData := range knowledge {
			if knowledgeItem, ok := knowledgeData.(map[string]interface{}); ok {
				if container, ok := knowledgeItem["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-knowledge-%s:%s", registry, agentName, knowledgeName, tag)
					}
				}
			}
		}
	}

	// Replace tools.*.container.build with tools.*.container.image
	if tools, ok := specObj["tools"].(map[string]interface{}); ok {
		for toolName, toolData := range tools {
			if tool, ok := toolData.(map[string]interface{}); ok {
				if container, ok := tool["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-tool-%s:%s", registry, agentName, toolName, tag)
					}
				}
			}
		}
	}

	// Replace interfaces.*.service.build with interfaces.*.service.image
	if interfaces, ok := specObj["interfaces"].(map[string]interface{}); ok {
		for ifaceName, ifaceData := range interfaces {
			if iface, ok := ifaceData.(map[string]interface{}); ok {
				if service, ok := iface["service"].(map[string]interface{}); ok {
					if _, hasBuild := service["build"]; hasBuild {
						delete(service, "build")
						service["image"] = fmt.Sprintf("%s/%s-interface-%s:%s", registry, agentName, ifaceName, tag)
					}
				}
			}
		}
	}

	return specObj
}
