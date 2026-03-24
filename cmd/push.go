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

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	spec "github.com/astropods/astro/packages/astro-spec"
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

Requires authentication (run the login command first).`,
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

	// Dev binary: skip push, default to native platform
	isDevBinary := binaryName == "ast-dev"
	if isDevBinary {
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
		return fmt.Errorf("registry URL required: run '%s login' or use --registry", binaryName)
	}

	if effectiveServerURL == "" && !skipRegister {
		return fmt.Errorf("server URL required for registration: run '%s login', use --server, or use --skip-register", binaryName)
	}

	// Parse Astro spec
	fmt.Printf("%s→%s Parsing %s\n", colorCyan, colorReset, filepath.Base(specPath))
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}
	warnDeprecatedMetaFields(specPath, workingDir)

	// Generate random build ID (8-char hex)
	pushTag = generateBuildID()

	// Build registry host from URL
	registryHost, err := getRegistryHost(effectiveRegistryURL)
	if err != nil {
		return fmt.Errorf("failed to parse registry URL: %w", err)
	}

	// Parse @account/name from spec — strip the prefix for all downstream use
	accountOverride, agentName := parseAgentName(astroSpec.Name)

	// Validate credentials upfront so stale tokens fail before build/push
	var orgToken string // non-empty when pushing to an organization
	if !noAuth {
		tokenManager := auth.NewTokenManager(binaryName)
		if !tokenManager.IsAuthenticated() {
			return fmt.Errorf("not authenticated. Run '%s login' to authenticate", binaryName)
		}
		if _, err := tokenManager.GetValidAccessToken(cmd.Context()); err != nil {
			return fmt.Errorf("authentication failed: %w. Run '%s login' to re-authenticate", err, binaryName)
		}
	}

	// Print header
	if accountOverride != "" {
		fmt.Printf("%s→%s Pushing %s%s%s to %s%s%s build %s\n\n", colorCyan, colorReset, colorBold, agentName, colorReset, colorCyan, accountOverride, colorReset, pushTag)
	} else {
		fmt.Printf("%s→%s Pushing %s%s%s build %s\n\n", colorCyan, colorReset, colorBold, agentName, colorReset, pushTag)
	}

	// Step 1: Get namespace from profile
	printStep("Reading account from profile...")
	namespace, workosOrgID, nsErr := getUserNamespace(verbose, accountOverride)
	if nsErr != nil {
		printStepFail()
		return fmt.Errorf("failed to get user namespace: %w", nsErr)
	}
	printStepDone(fmt.Sprintf("namespace: %s", namespace))

	// If pushing to an org, obtain an org-scoped token
	if workosOrgID != "" && !noAuth {
		printStep("Obtaining organization token...")
		tokenManager := auth.NewTokenManager(binaryName)
		var tokenErr error
		orgToken, tokenErr = tokenManager.GetOrgScopedAccessToken(cmd.Context(), workosOrgID)
		if tokenErr != nil {
			printStepFail()
			return fmt.Errorf("failed to get org-scoped token: %w", tokenErr)
		}
		printStepDone("")
	}

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
			baseName := agentName
			remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

			printPushStart("agent", baseName)
			size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth, orgToken)
			if err != nil {
				printPushComplete(false, 0)
				return fmt.Errorf("failed to push agent image: %w", err)
			}
			printPushComplete(true, size)
			imagesPushed++
		} else {
			// Single platform: push the platform-specific image we built (not the convenience tag)
			localImageName := platformImageTag(agentName, pushTag, platforms[0])
			remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, agentName, pushTag)

			printPushStart("agent", agentName)
			size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth, orgToken)
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
				baseName := fmt.Sprintf("%s-model-%s", agentName, modelName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("model", modelName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth, orgToken)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push model %s: %w", modelName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth, orgToken)
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
				baseName := fmt.Sprintf("%s-knowledge-%s", agentName, knowledgeName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("knowledge", knowledgeName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth, orgToken)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push knowledge store %s: %w", knowledgeName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth, orgToken)
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
				baseName := fmt.Sprintf("%s-tool-%s", agentName, toolName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("tool", toolName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth, orgToken)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push tool %s: %w", toolName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth, orgToken)
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
				baseName := fmt.Sprintf("%s-ingestion-%s", agentName, ingestionName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, baseName, pushTag)

				printPushStart("ingestion", ingestionName)
				if multiPlatform {
					size, err := pushMultiPlatformToRegistryStreaming(baseName, pushTag, remoteImageName, platforms, noAuth, orgToken)
					if err != nil {
						printPushComplete(false, 0)
						return fmt.Errorf("failed to push ingestion %s: %w", ingestionName, err)
					}
					printPushComplete(true, size)
				} else {
					localImageName := platformImageTag(baseName, pushTag, platforms[0])
					size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, noAuth, orgToken)
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
		if isDevBinary {
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
				platformImageTag(agentName, pushTag, platform),
				fmt.Sprintf("%s/%s/%s:%s", registryHost, namespace, agentName, pushTag),
			); err != nil {
				return err
			}

			// Custom-built model images
			for modelName, model := range astroSpec.Models {
				if model.Container != nil && model.Container.Build != nil {
					baseName := fmt.Sprintf("%s-model-%s", agentName, modelName)
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
					baseName := fmt.Sprintf("%s-knowledge-%s", agentName, knowledgeName)
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
					baseName := fmt.Sprintf("%s-tool-%s", agentName, toolName)
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
					baseName := fmt.Sprintf("%s-ingestion-%s", agentName, ingestionName)
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
		// Read AGENT.md if it exists (agent card file)
		readmeContent := ""
		readmePath := filepath.Join(workingDir, "AGENT.md")
		if readmeData, err := os.ReadFile(readmePath); err == nil { //nolint:gosec
			readmeContent = string(readmeData)
		}

		// Determine visibility
		visibility := astroSpec.Meta.Visibility
		if visibility != "public" && visibility != "private" {
			visibility = "" // ignore invalid values
		}

		// Default to private when not set in spec
		if visibility == "" {
			visibility = "private"
		}

		// If the agent already exists with a different visibility, confirm the change
		serverAgent := getAgentFromServer(effectiveServerURL, namespace, agentName, noAuth, orgToken)
		if serverAgent.Exists && serverAgent.Visibility != "" && visibility != serverAgent.Visibility {
			if !confirmVisibilityChange(serverAgent.Visibility, visibility) {
				visibility = serverAgent.Visibility // keep current visibility
			}
		}

		printStep("Registering agent with server...")

		// Build the full registry path for the transformed spec
		registryPath := fmt.Sprintf("%s/%s", registryHost, namespace)
		if err := registerAgent(effectiveServerURL, agentName, pushTag, registryPath, specPath, pushTag, readmeContent, visibility, verbose, noAuth, orgToken); err != nil {
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
	fmt.Printf("  %s%s%s tag %s%s%s\n\n", colorCyan, agentName, colorReset, colorDim, pushTag, colorReset)

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

// parseAgentName splits a spec name into an optional account override and the bare agent name.
// "@my-org/my-agent" → ("my-org", "my-agent")
// "my-agent"         → ("", "my-agent")
func parseAgentName(raw string) (account, name string) {
	if strings.HasPrefix(raw, "@") {
		trimmed := strings.TrimPrefix(raw, "@")
		if idx := strings.Index(trimmed, "/"); idx > 0 && idx < len(trimmed)-1 {
			return trimmed[:idx], trimmed[idx+1:]
		}
	}
	return "", raw
}

// getUserNamespace reads the user's namespace (account name) from the stored profile.
// If accountOverride is non-empty, it looks up that account and returns its WorkOS org ID
// (needed for org-scoped token refresh). Otherwise it returns the personal account.
func getUserNamespace(verbose bool, accountOverride string) (namespace, workosOrgID string, err error) {
	storage := auth.NewStorage(binaryName)
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return "", "", fmt.Errorf("not logged in. Run '%s login' to authenticate", binaryName)
	}

	if accountOverride != "" {
		// Look up the requested account
		for _, acct := range profile.Accounts {
			if strings.EqualFold(acct.Name, accountOverride) {
				if acct.Type == "organization" && acct.WorkOSOrganizationID == "" {
					return "", "", fmt.Errorf("organization %q is not linked. Run '%s login' to refresh your accounts", accountOverride, binaryName)
				}
				if verbose {
					fmt.Fprintf(os.Stderr, "  %sAccount: %s (ID: %s, type: %s)%s\n", colorDim, acct.Name, acct.ID, acct.Type, colorReset)
				}
				return strings.ToLower(acct.Name), acct.WorkOSOrganizationID, nil
			}
		}
		// Not found — list available accounts
		var names []string
		for _, acct := range profile.Accounts {
			names = append(names, acct.Name)
		}
		return "", "", fmt.Errorf("account %q not found. Available accounts: %s. Run '%s login' to refresh", accountOverride, strings.Join(names, ", "), binaryName)
	}

	// Default: personal account
	name := profile.User.AccountName
	if name == "" && len(profile.Accounts) > 0 {
		name = profile.Accounts[0].Name
	}

	if name == "" {
		return "", "", fmt.Errorf("no account found. Visit the dashboard to choose your username, then run '%s login' again", binaryName)
	}

	if verbose {
		accountID := profile.User.AccountID
		if accountID == "" && len(profile.Accounts) > 0 {
			accountID = profile.Accounts[0].ID
		}
		fmt.Fprintf(os.Stderr, "  %sAccount: %s (ID: %s)%s\n", colorDim, name, accountID, colorReset)
	}

	return strings.ToLower(name), "", nil
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
func registerAgent(serverURL, agentName, buildID, registry, specPath, pushTag, readme, visibility string, verbose bool, skipAuth bool, tokenOverride string) error {
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

	// Strip default values from secret inputs so credentials are not stored in the registry
	stripSecretDefaults(specObj)

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
	if visibility != "" {
		payload["visibility"] = visibility
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
	if verbose {
		log.Printf("   Register URL: %s", reqURL)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, reqURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cli-Version", version)

	// Add authentication header if not skipped
	if !skipAuth {
		if tokenOverride != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenOverride))
		} else if err := auth.AddAuthHeader(context.Background(), req, binaryName); err != nil {
			return fmt.Errorf("failed to add authentication: %w. Run '%s login' to re-authenticate", err, binaryName)
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

	if resp.StatusCode == http.StatusUpgradeRequired {
		body, _ := io.ReadAll(resp.Body)
		var errResp map[string]interface{}
		if json.Unmarshal(body, &errResp) == nil {
			if msg, ok := errResp["error"].(string); ok {
				return fmt.Errorf("%s\nRun '%s upgrade' to update", msg, binaryName)
			}
		}
		return fmt.Errorf("CLI version %s is too old. Run '%s upgrade' to update", version, binaryName)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed (401). Server response: %s\nRun '%s login' to re-authenticate", string(body), binaryName)
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

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if verbose {
			log.Printf("   Server response: %v", result) //nolint:gosec
		}
		if hints, ok := result["hints"].([]interface{}); ok {
			for _, h := range hints {
				if s, ok := h.(string); ok {
					fmt.Fprintf(os.Stderr, "%s⚠%s  %s\n", colorYellow, colorReset, s)
				}
			}
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
)

// colorCyan uses the primary accent color from the theme (teal in prod, pink in preview).
var colorCyan = theme.PrimaryANSI

// warnDeprecatedMetaFields reads a spec file and prints deprecation warnings
// for meta.description and meta.tags that have moved to AGENT.md frontmatter.
// It also warns if no AGENT.md file exists in the working directory.
func warnDeprecatedMetaFields(specPath, workingDir string) {
	data, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return
	}
	for _, msg := range spec.DeprecatedMetaFields(data) {
		fmt.Fprintf(os.Stderr, "%s⚠%s  %s\n", colorYellow, colorReset, msg)
	}

	agentMdPath := filepath.Join(workingDir, "AGENT.md")
	if _, err := os.Stat(agentMdPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s⚠%s  No AGENT.md found — create one next to your astropods.yml to make your agent more discoverable\n", colorYellow, colorReset)
	}
}

// agentServerInfo holds metadata about an agent fetched from the server.
type agentServerInfo struct {
	Exists     bool
	Visibility string
}

// getAgentFromServer checks if an agent exists on the server and returns its metadata.
func getAgentFromServer(serverURL, accountName, agentName string, skipAuth bool, tokenOverride string) agentServerInfo {
	reqURL := fmt.Sprintf("%s/api/v1/agents/%s/%s",
		strings.TrimSuffix(serverURL, "/"),
		url.PathEscape(accountName),
		url.PathEscape(agentName),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s⚠%s  Could not check agent status: %v\n", colorYellow, colorReset, err)
		return agentServerInfo{}
	}

	if !skipAuth {
		if tokenOverride != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenOverride))
		} else if err := auth.AddAuthHeader(context.Background(), req, binaryName); err != nil {
			fmt.Fprintf(os.Stderr, "%s⚠%s  Could not check agent status: auth error\n", colorYellow, colorReset)
			return agentServerInfo{}
		}
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s⚠%s  Could not check agent status: %v\n", colorYellow, colorReset, err)
		return agentServerInfo{}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return agentServerInfo{}
	}

	var body struct {
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return agentServerInfo{Exists: true}
	}
	return agentServerInfo{Exists: true, Visibility: body.Visibility}
}

// confirmVisibilityChange asks the user to confirm a visibility change from current to desired.
func confirmVisibilityChange(current, desired string) bool {
	var confirmed bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Change visibility from %s to %s?", current, desired)).
				Description(fmt.Sprintf("This agent is currently %s. Your spec sets visibility to %s.", current, desired)).
				Value(&confirmed),
		),
	)

	huhTheme := huh.ThemeCharm()
	primary := theme.Primary
	huhTheme.Focused.Title = huhTheme.Focused.Title.Foreground(primary)
	form.WithTheme(huhTheme)

	if err := form.Run(); err != nil {
		return false
	}

	return confirmed
}

// promptVisibility asks the user whether the agent should be public or private.
func promptVisibility() string { //nolint:unused
	var visibility string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Agent visibility").
				Description("Public agents are visible to everyone. Private agents are only visible to account members.").
				Options(
					huh.NewOption("Public", "public"),
					huh.NewOption("Private", "private"),
				).
				Value(&visibility),
		),
	)

	huhTheme := huh.ThemeCharm()
	primary := theme.Primary
	huhTheme.Focused.Title = huhTheme.Focused.Title.Foreground(primary)
	huhTheme.Focused.SelectedOption = huhTheme.Focused.SelectedOption.Foreground(primary)
	huhTheme.Focused.SelectedPrefix = huhTheme.Focused.SelectedPrefix.Foreground(primary)
	form.WithTheme(huhTheme)

	if err := form.Run(); err != nil {
		// Default to private if prompt fails (e.g. non-interactive terminal)
		return "private"
	}

	return visibility
}

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

// stripSecretDefaults removes default values from secret inputs across all spec
// sections so credentials are not stored in the registry. Operates on the raw
// map representation (same pattern as transformSpecForRegistry).
func stripSecretDefaults(specObj map[string]interface{}) {
	// Top-level inputs (map[string]input)
	if inputs, ok := specObj["inputs"].(map[string]interface{}); ok {
		for _, inputData := range inputs {
			stripSecretInputDefault(inputData)
		}
	}

	// Agent inputs (list)
	if agent, ok := specObj["agent"].(map[string]interface{}); ok {
		stripSecretInputList(agent["inputs"])
	}

	// Models/knowledge/tools/ingestion — each entry may have an inputs list
	for _, section := range []string{"models", "knowledge", "tools", "ingestion"} {
		if entries, ok := specObj[section].(map[string]interface{}); ok {
			for _, entryData := range entries {
				if entry, ok := entryData.(map[string]interface{}); ok {
					stripSecretInputList(entry["inputs"])
				}
			}
		}
	}

	// Providers — variables list
	if providers, ok := specObj["providers"].(map[string]interface{}); ok {
		for _, provData := range providers {
			if prov, ok := provData.(map[string]interface{}); ok {
				stripSecretInputList(prov["variables"])
			}
		}
	}
}

// stripSecretInputList strips defaults from a YAML list of inputs ([]interface{}).
func stripSecretInputList(v interface{}) {
	list, ok := v.([]interface{})
	if !ok {
		return
	}
	for _, item := range list {
		stripSecretInputDefault(item)
	}
}

// stripSecretInputDefault removes the "default" field from a single input map
// if it has secret: true.
func stripSecretInputDefault(v interface{}) {
	input, ok := v.(map[string]interface{})
	if !ok {
		return
	}
	if secret, _ := input["secret"].(bool); secret {
		delete(input, "default")
	}
}
