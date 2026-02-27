package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
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

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/postman/astro/apps/astro-cli/internal/auth"
	spec "github.com/postman/astro/packages/astro-spec"
)

// getOptimizedTransport returns an HTTP transport optimized for large file uploads
// This explicitly disables HTTP/2 to ensure compatibility with all registries
func getOptimizedTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90,
		DisableCompression:  false,
		// Explicitly disable HTTP/2 by setting TLSNextProto to empty map
		// This prevents "http2: client conn could not be established" errors
		// with registries that have incomplete or misconfigured HTTP/2 support
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push agent images and spec to astro platform",
	Long: `Push container images to astro-registry and spec to astro-server.

This will:
1. Generate a build ID
2. Tag and push the agent container image to astro-registry
3. Tag and push custom-built component images (models, knowledge, tools)
4. Register the agent spec with astro-server

Images are pushed through the astro-registry service which proxies to ECR.
The spec is registered with astro-server which validates and stores it.

Example:
  ast push
  ast push --build

Requirements:
  - Must be authenticated (run 'ast login' first)
  - Server and registry URLs come from your login profile (set via 'ast login')`,
	RunE: runPush,
}

var (
	pushTag      string // random build ID generated at push time
	skipBuild    bool
	skipPush     bool
	serverURL    string
	registryURL  string
	skipRegister bool
	noAuth       bool
	pushPlatform string
	pushLocal    bool
)

func init() {
	rootCmd.AddCommand(pushCmd)
	pushCmd.Flags().BoolVar(&skipBuild, "skip-build", false, "Skip building before pushing")
	pushCmd.Flags().BoolVar(&skipPush, "skip-push", false, "Skip pushing images to registry")
	pushCmd.Flags().StringVar(&serverURL, "server", "", "Astro server URL (overrides profile/default)")
	pushCmd.Flags().StringVar(&registryURL, "registry", "", "Astro registry URL (default: registry.<server-host>)")
	pushCmd.Flags().BoolVar(&skipRegister, "skip-register", false, "Skip registering agent spec with server")
	pushCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Skip authentication (not recommended)")
	pushCmd.Flags().StringVar(&pushPlatform, "platform", "linux/amd64", "Target platform(s) for push (comma-separated)")
	pushCmd.Flags().BoolVar(&pushLocal, "local", false, "Build and register with locally running astro-server (skip registry push)")
	_ = pushCmd.Flags().MarkHidden("local")
}

func runPush(cmd *cobra.Command, args []string) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	specPath, err := resolveSpecPath(cmd, workingDir)
	if err != nil {
		return err
	}
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Get server URL: --server flag > build-time default
	effectiveServerURL := serverURL
	if effectiveServerURL == "" {
		effectiveServerURL = auth.DefaultServerURL
	}

	// Local mode: skip push, default to native platform
	if pushLocal {
		skipPush = true
		if !cmd.Flags().Changed("platform") {
			pushPlatform = nativePlatform()
		}
	}

	// Get registry URL: --registry flag > derived from server URL
	effectiveRegistryURL := registryURL
	if effectiveRegistryURL == "" {
		effectiveRegistryURL = auth.RegistryURLFromServerURL(effectiveServerURL)
	}

	if verbose {
		fmt.Printf("%s→%s Server URL:   %s%s%s\n", colorCyan, colorReset, colorDim, effectiveServerURL, colorReset)
		fmt.Printf("%s→%s Registry URL: %s%s%s\n", colorCyan, colorReset, colorDim, effectiveRegistryURL, colorReset)
	}

	if effectiveRegistryURL == "" {
		return fmt.Errorf("registry URL required: run 'ast login' or use --registry")
	}

	if effectiveServerURL == "" && !skipRegister {
		return fmt.Errorf("server URL required for registration: run 'ast login', use --server, or use --skip-register")
	}

	// Parse Astro spec
	fmt.Printf("%s→%s Parsing %s\n", colorCyan, colorReset, filepath.Base(specPath))
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	// Generate random build ID (8-char hex)
	pushTag = generateBuildID()

	// Build registry host from URL
	registryHost, err := getRegistryHost(effectiveRegistryURL)
	if err != nil {
		return fmt.Errorf("failed to parse registry URL: %w", err)
	}

	// Check authentication
	if !noAuth {
		tokenManager := auth.NewTokenManager(binaryName)
		if !tokenManager.IsAuthenticated() {
			return fmt.Errorf("not authenticated. Run 'ast login' to authenticate")
		}
	}

	// Print header
	fmt.Printf("%s→%s Pushing %s%s%s build %s\n\n", colorCyan, colorReset, colorBold, astroSpec.Name, colorReset, pushTag)

	// Step 1: Get namespace from profile
	printStep("Reading account from profile...")
	namespace, nsErr := getUserNamespace(verbose)
	if nsErr != nil {
		printStepFail()
		return fmt.Errorf("failed to get user namespace: %w", nsErr)
	}
	printStepDone(fmt.Sprintf("namespace: %s", namespace))

	// Build images first if requested
	imagesPushed := 0
	platforms := parsePlatforms(pushPlatform)
	multiPlatform := len(platforms) > 1

	if !skipBuild {
		// Use the same tag for build so built images match what we push
		buildTag = pushTag
		buildPlatform = pushPlatform
		printStep("Building images")
		fmt.Println()
		if err := runBuild(cmd, args); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}
	}

	// Push images
	if !skipPush {
		// 1. Push agent container image
		if multiPlatform {
			baseName := astroSpec.Name
			remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

			printPushStart("agent", baseName)
			size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push agent image: %w", err)
			}
			printPushComplete(true, size)
			imagesPushed++
		} else {
			// Single platform: push the platform-specific image we built (not the convenience tag)
			localImageName := platformImageTag(astroSpec.Name, pushTag, platforms[0])
			remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, astroSpec.Name, pushTag)

			printPushStart("agent", astroSpec.Name)
			size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push agent image: %w", err)
			}
			printPushComplete(true, size)
			imagesPushed++
		}

		// 2. Push custom-built model images
		for modelName, model := range astroSpec.Models {
			if model.Container != nil && model.Container.Build != nil {
				baseName := fmt.Sprintf("%s-model-%s", astroSpec.Name, modelName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("model", modelName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push model %s: %w", modelName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push model %s: %w", modelName, err)
					}
					printPushComplete(true, size)
				}
				imagesPushed++
			}
		}

		// 3. Push custom-built knowledge store images
		for knowledgeName, knowledge := range astroSpec.Knowledge {
			container := knowledge.ResolvedContainer()
			if container.Build != nil {
				baseName := fmt.Sprintf("%s-knowledge-%s", astroSpec.Name, knowledgeName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("knowledge", knowledgeName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push knowledge store %s: %w", knowledgeName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push knowledge store %s: %w", knowledgeName, err)
					}
					printPushComplete(true, size)
				}
				imagesPushed++
			}
		}

		// 4. Push custom-built tool images
		for toolName, tool := range astroSpec.Tools {
			if tool.Container != nil && tool.Container.Build != nil {
				baseName := fmt.Sprintf("%s-tool-%s", astroSpec.Name, toolName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("tool", toolName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push tool %s: %w", toolName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push tool %s: %w", toolName, err)
					}
					printPushComplete(true, size)
				}
				imagesPushed++
			}
		}

		// 5. Push custom-built ingestion images
		for ingestionName, ingestion := range astroSpec.Ingestion {
			if ingestion.Container.Build != nil {
				baseName := fmt.Sprintf("%s-ingestion-%s", astroSpec.Name, ingestionName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("ingestion", ingestionName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push ingestion %s: %w", ingestionName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push ingestion %s: %w", ingestionName, err)
					}
					printPushComplete(true, size)
				}
				imagesPushed++
			}
		}

	} else {
		fmt.Printf("%s→%s Skipping image push %s(--skip-push)%s\n", colorCyan, colorReset, colorDim, colorReset)

		// Retag locally-built platform images to registry paths so the local server can resolve them
		if pushLocal {
			retag := func(local, remote string) error {
				cmd := exec.Command("docker", "tag", local, remote) //nolint:gosec
				if out, err := cmd.CombinedOutput(); err != nil {
					return fmt.Errorf("failed to retag %s → %s: %s", local, remote, strings.TrimSpace(string(out)))
				}
				fmt.Printf("  %s✓%s %s%s%s\n", colorGreen, colorReset, colorDim, remote, colorReset)
				return nil
			}

			platform := platforms[0]

			// Agent image
			if err := retag(
				platformImageTag(astroSpec.Name, pushTag, platform),
				fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, astroSpec.Name, pushTag),
			); err != nil {
				return err
			}

			// Custom-built model images
			for modelName, model := range astroSpec.Models {
				if model.Container != nil && model.Container.Build != nil {
					baseName := fmt.Sprintf("%s-model-%s", astroSpec.Name, modelName)
					if err := retag(
						platformImageTag(baseName, pushTag, platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag),
					); err != nil {
						return err
					}
				}
			}

			// Custom-built knowledge images
			for knowledgeName, knowledge := range astroSpec.Knowledge {
				container := knowledge.ResolvedContainer()
				if container.Build != nil {
					baseName := fmt.Sprintf("%s-knowledge-%s", astroSpec.Name, knowledgeName)
					if err := retag(
						platformImageTag(baseName, pushTag, platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag),
					); err != nil {
						return err
					}
				}
			}

			// Custom-built tool images
			for toolName, tool := range astroSpec.Tools {
				if tool.Container != nil && tool.Container.Build != nil {
					baseName := fmt.Sprintf("%s-tool-%s", astroSpec.Name, toolName)
					if err := retag(
						platformImageTag(baseName, pushTag, platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag),
					); err != nil {
						return err
					}
				}
			}

			// Custom-built ingestion images
			for ingestionName, ingestion := range astroSpec.Ingestion {
				if ingestion.Container.Build != nil {
					baseName := fmt.Sprintf("%s-ingestion-%s", astroSpec.Name, ingestionName)
					if err := retag(
						platformImageTag(baseName, pushTag, platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag),
					); err != nil {
						return err
					}
				}
			}
		}
	}

	fmt.Println()

	// Register agent spec with server
	if !skipRegister && effectiveServerURL != "" {
		printStep("Registering agent with server...")

		// Read README.md if it exists
		readmeContent := ""
		readmePath := filepath.Join(workingDir, "README.md")
		if readmeData, err := os.ReadFile(readmePath); err == nil { //nolint:gosec
			readmeContent = string(readmeData)
		}

		// Build the full registry path for the transformed spec
		registryPath := fmt.Sprintf("%s/%s", registryHost, namespace)
		if err := registerAgent(effectiveServerURL, astroSpec.Name, pushTag, registryPath, specPath, pushTag, readmeContent, verbose, noAuth); err != nil {
			printStepFail()
			return fmt.Errorf("registration failed: %w", err)
		} else {
			printStepDone("")
		}
	} else if skipRegister {
		printStep("Registering agent with server")
		fmt.Printf(" %s(skipped)%s\n", colorDim, colorReset)
	}

	// Final summary
	fmt.Printf("\n%s%s✓ Pushed successfully!%s\n", colorBold, colorGreen, colorReset)
	fmt.Printf("  %s%s%s tag %s%s%s\n\n", colorCyan, astroSpec.Name, colorReset, colorDim, pushTag, colorReset)

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

// getUserNamespace reads the user's namespace (account name) from the stored profile.
func getUserNamespace(verbose bool) (string, error) {
	storage := auth.NewStorage(binaryName)
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return "", fmt.Errorf("not logged in. Run 'ast login' to authenticate")
	}

	// Try stored account name
	name := profile.User.AccountName
	if name == "" && len(profile.Accounts) > 0 {
		name = profile.Accounts[0].Name
	}

	if name == "" {
		return "", fmt.Errorf("no account found. Visit the dashboard to choose your username, then run 'ast login' again")
	}

	if verbose {
		accountID := profile.User.AccountID
		if accountID == "" && len(profile.Accounts) > 0 {
			accountID = profile.Accounts[0].ID
		}
		fmt.Fprintf(os.Stderr, "  %sAccount: %s (ID: %s)%s\n", colorDim, name, accountID, colorReset)
	}

	return strings.ToLower(name), nil
}

// generateBuildID returns a random 8-character hex string (4 bytes).
func generateBuildID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate build ID: %v", err))
	}
	return hex.EncodeToString(b)
}

// getRegistryHost extracts the host from the registry URL for use as registry address.
func getRegistryHost(registryURL string) (string, error) {
	u, err := url.Parse(registryURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

// registerAgent registers the agent spec with the astro-server
func registerAgent(serverURL, agentName, buildID, registry, specPath, pushTag, readme string, verbose bool, skipAuth bool) error {
	// Read and parse spec file
	specData, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	// Parse YAML spec
	var specObj map[string]interface{}
	if err := yaml.Unmarshal(specData, &specObj); err != nil {
		return fmt.Errorf("failed to parse spec YAML: %w", err)
	}

	// Transform spec: replace build sections with actual image references
	specObj = transformSpecForRegistry(specObj, registry, agentName, pushTag)

	// Marshal back to YAML
	transformedSpecData, err := yaml.Marshal(specObj)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed spec: %w", err)
	}

	// Extract account name from registry path (registryHost/accountName)
	accountName := ""
	registryParts := strings.Split(registry, "/")
	if len(registryParts) >= 2 {
		accountName = registryParts[len(registryParts)-1]
	}

	// Prepare request payload (name comes from URL now, not body)
	payload := map[string]string{
		"build_id":     buildID,
		"registry":     registry,
		"spec_content": string(transformedSpecData),
		"readme":       readme,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send POST request to account-scoped registration endpoint
	reqURL := fmt.Sprintf("%s/api/v1/agents/%s/%s/register",
		strings.TrimSuffix(serverURL, "/"),
		url.PathEscape(accountName),
		url.PathEscape(agentName),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication header if not skipped
	if !skipAuth {
		if err := auth.AddAuthHeader(context.Background(), req, binaryName); err != nil {
			return fmt.Errorf("failed to add authentication: %w. Run 'astro login' to re-authenticate", err)
		}
		if verbose {
			authHeader := req.Header.Get("Authorization")
			if authHeader != "" {
				// Show first/last few chars of token for debugging
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if len(token) > 20 {
					log.Printf("   Auth: Bearer %s...%s (len=%d)", token[:10], token[len(token)-5:], len(token)) //nolint:gosec
				} else {
					log.Printf("   Auth: Bearer <short token, len=%d>", len(token)) //nolint:gosec
				}
			} else {
				log.Printf("   Auth: WARNING - no Authorization header set!") //nolint:gosec
			}
		}
	}

	client := &http.Client{
		// Don't follow redirects — a 301/302 redirect downgrades POST to GET,
		// which causes the request to hit GET /agents/:name instead of POST /agents/register.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// If the server redirected, report it clearly instead of silently failing
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		hint := ""
		if strings.HasPrefix(reqURL, "http://") && strings.HasPrefix(location, "https://") {
			hint = ". It looks like the server requires HTTPS — try updating your server URL to use https://"
		}
		return fmt.Errorf("server returned redirect (%d) to %q%s", resp.StatusCode, location, hint)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed (401). Server response: %s\nRun 'ast login' to re-authenticate", string(body))
	}

	if resp.StatusCode != http.StatusCreated {
		// Read the full response body for detailed error logging
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("server returned status %d (failed to read response body: %w)", resp.StatusCode, readErr)
		}

		// Log the raw error response
		log.Printf("Registration failed with status %d. Response body: %s", resp.StatusCode, string(body)) //nolint:gosec

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
			log.Printf("   Server response: %v", result) //nolint:gosec
		}
	}

	return nil
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
)

// transformSpecForRegistry replaces build sections with actual image references
func transformSpecForRegistry(specObj map[string]interface{}, registry, agentName, tag string) map[string]interface{} {
	// Replace agent.build with agent.image
	if agent, ok := specObj["agent"].(map[string]interface{}); ok {
		if _, hasBuild := agent["build"]; hasBuild {
			delete(agent, "build")
			agent["image"] = fmt.Sprintf("%s/%s:%s", registry, agentName, tag)
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

	// Replace ingestion.*.container.build with ingestion.*.container.image
	if ingestion, ok := specObj["ingestion"].(map[string]interface{}); ok {
		for ingestionName, ingestionData := range ingestion {
			if ingestionItem, ok := ingestionData.(map[string]interface{}); ok {
				if container, ok := ingestionItem["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-ingestion-%s:%s", registry, agentName, ingestionName, tag)
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
