package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/moby/moby/client"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// pushServerURLOverride is set in tests to redirect API calls to a test server.
var pushServerURLOverride string

func pushBaseURL() string {
	if pushServerURLOverride != "" {
		return strings.TrimSuffix(pushServerURLOverride, "/")
	}
	return strings.TrimSuffix(auth.DefaultServerURL, "/")
}

// pushConfig holds all parameters for a push operation.
type pushConfig struct {
	specPath   string
	agentName  string
	skipBuild  bool
	skipPush   bool
	platform   string
	visibility Visibility // VisibilityPublic, VisibilityPrivate, or VisibilityUnset (preserve existing)
	yes        bool       // skip interactive confirmation prompts
	verbose    bool
}

// runPush assumes the spec in cfg.specPath is valid; callers must validate before invoking.
func runPush(ctx context.Context, at AccountToken, cfg pushConfig) error {
	workingDir := filepath.Dir(cfg.specPath)

	effectiveServerURL := pushBaseURL()
	effectiveRegistryURL := auth.RegistryURLFromServerURL(effectiveServerURL)

	if cfg.verbose {
		fmt.Printf("%s→%s Server URL:   %s%s%s\n", colorCyan, colorReset, colorDim, effectiveServerURL, colorReset)
		fmt.Printf("%s→%s Registry URL: %s%s%s\n", colorCyan, colorReset, colorDim, effectiveRegistryURL, colorReset)
	}

	if effectiveRegistryURL == "" {
		return fmt.Errorf("registry URL required: run '%s login'", binaryName)
	}

	if effectiveServerURL == "" {
		return fmt.Errorf("server URL required: run '%s login'", binaryName)
	}

	astroSpec, err := spec.ParseSpec(cfg.specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}
	warnDeprecatedMetaFields(cfg.specPath, workingDir)

	tag := generateBuildID()

	registryHost, err := getRegistryHost(effectiveRegistryURL)
	if err != nil {
		return fmt.Errorf("failed to parse registry URL: %w", err)
	}

	agentName := cfg.agentName

	fmt.Printf("%s→%s Pushing %s%s%s to %s%s%s build %s\n\n", colorCyan, colorReset, colorBold, agentName, colorReset, colorCyan, at.Account, colorReset, tag)

	imagesPushed := 0

	if !cfg.skipBuild {
		printStep("Building images")
		fmt.Println()
		if err := runBuild(ctx, cfg.specPath, agentName, tag, []string{cfg.platform}, false, cfg.verbose, false); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}
	}

	if !cfg.skipPush {
		localImageName := platformImageTag(agentName, tag, cfg.platform)
		remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, agentName, tag)

		printPushStart("agent", agentName)
		size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, false, at.Token)
		if err != nil {
			printPushComplete(false, 0)
			return fmt.Errorf("failed to push agent image: %w", err)
		}
		printPushComplete(true, size)
		imagesPushed++

		for modelName, model := range astroSpec.Models {
			if model.Container != nil && model.Container.Build != nil {
				baseName := fmt.Sprintf("%s-model-%s", agentName, modelName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag)

				printPushStart("model", modelName)
				localImageName := platformImageTag(baseName, tag, cfg.platform)
				size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, false, at.Token)
				if err != nil {
					printPushComplete(false, 0)
					return fmt.Errorf("failed to push model %s: %w", modelName, err)
				}
				printPushComplete(true, size)

				imagesPushed++
			}
		}

		for knowledgeName, knowledge := range astroSpec.Knowledge {
			container := knowledge.ResolvedContainer()
			if container.Build != nil {
				baseName := fmt.Sprintf("%s-knowledge-%s", agentName, knowledgeName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag)

				printPushStart("knowledge", knowledgeName)
				localImageName := platformImageTag(baseName, tag, cfg.platform)
				size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, false, at.Token)
				if err != nil {
					printPushComplete(false, 0)
					return fmt.Errorf("failed to push knowledge store %s: %w", knowledgeName, err)
				}
				printPushComplete(true, size)
				imagesPushed++
			}
		}

		for toolName, tool := range astroSpec.Integrations {
			if tool.Container != nil && tool.Container.Build != nil {
				baseName := fmt.Sprintf("%s-tool-%s", agentName, toolName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag)

				printPushStart("integration", toolName)
				localImageName := platformImageTag(baseName, tag, cfg.platform)
				size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, false, at.Token)
				if err != nil {
					printPushComplete(false, 0)
					return fmt.Errorf("failed to push integration %s: %w", toolName, err)
				}
				printPushComplete(true, size)
				imagesPushed++
			}
		}

		for ingestionName, ingestion := range astroSpec.Ingestion {
			if ingestion.Container.Build != nil {
				baseName := fmt.Sprintf("%s-ingestion-%s", agentName, ingestionName)
				remoteImageName := fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag)

				printPushStart("ingestion", ingestionName)
				localImageName := platformImageTag(baseName, tag, cfg.platform)
				size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, false, at.Token)
				if err != nil {
					printPushComplete(false, 0)
					return fmt.Errorf("failed to push ingestion %s: %w", ingestionName, err)
				}
				printPushComplete(true, size)
				imagesPushed++
			}
		}
	} else {
		fmt.Printf("%s→%s Skipping image push %s(local dev server detected)%s\n", colorCyan, colorReset, colorDim, colorReset)

		if cfg.skipPush && !cfg.skipBuild {
			dockerCli, err := newDockerClient()
			if err != nil {
				return err
			}

			retag := func(local, remote string) error {
				if _, err := dockerCli.ImageTag(ctx, client.ImageTagOptions{Source: local, Target: remote}); err != nil {
					return fmt.Errorf("failed to retag %s → %s: %w", local, remote, err)
				}
				fmt.Printf("  %s✓%s %s%s%s\n", colorGreen, colorReset, colorDim, remote, colorReset)
				return nil
			}

			if err := retag(
				platformImageTag(agentName, tag, cfg.platform),
				fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, agentName, tag),
			); err != nil {
				return err
			}

			for modelName, model := range astroSpec.Models {
				if model.Container != nil && model.Container.Build != nil {
					baseName := fmt.Sprintf("%s-model-%s", agentName, modelName)
					if err := retag(
						platformImageTag(baseName, tag, cfg.platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag),
					); err != nil {
						return err
					}
				}
			}

			for knowledgeName, knowledge := range astroSpec.Knowledge {
				container := knowledge.ResolvedContainer()
				if container.Build != nil {
					baseName := fmt.Sprintf("%s-knowledge-%s", agentName, knowledgeName)
					if err := retag(
						platformImageTag(baseName, tag, cfg.platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag),
					); err != nil {
						return err
					}
				}
			}

			for toolName, tool := range astroSpec.Integrations {
				if tool.Container != nil && tool.Container.Build != nil {
					baseName := fmt.Sprintf("%s-tool-%s", agentName, toolName)
					if err := retag(
						platformImageTag(baseName, tag, cfg.platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag),
					); err != nil {
						return err
					}
				}
			}

			for ingestionName, ingestion := range astroSpec.Ingestion {
				if ingestion.Container.Build != nil {
					baseName := fmt.Sprintf("%s-ingestion-%s", agentName, ingestionName)
					if err := retag(
						platformImageTag(baseName, tag, cfg.platform),
						fmt.Sprintf("%s/%s/%s:%s", registryHost, at.Account, baseName, tag),
					); err != nil {
						return err
					}
				}
			}
		}
	}

	fmt.Println()

	visibility := VisibilityPrivate
	if cfg.visibility != VisibilityUnset {
		visibility = cfg.visibility
	}

	readmeContent := ""
	readmePath := filepath.Join(workingDir, "AGENT.md")
	if readmeData, err := os.ReadFile(readmePath); err == nil { //nolint:gosec
		readmeContent = string(readmeData)
	}

	serverAgent := getAgentFromServer(effectiveServerURL, at.Account, agentName, false, at.Token)

	if serverAgent.Exists && serverAgent.Visibility == string(VisibilityPublic) && cfg.visibility != VisibilityPrivate {
		visibility = VisibilityPublic
	}

	needsConfirm := (visibility == VisibilityPublic && (!serverAgent.Exists || serverAgent.Visibility != string(VisibilityPublic))) ||
		(cfg.visibility == VisibilityPrivate && serverAgent.Exists && serverAgent.Visibility == string(VisibilityPublic))
	if needsConfirm && !cfg.yes {
		if !confirmVisibilityChange(serverAgent.Visibility, string(visibility)) {
			return fmt.Errorf("push cancelled")
		}
	}

	printStep("Registering agent with server...")
	registryPath := fmt.Sprintf("%s/%s", registryHost, at.Account)
	if err := registerAgent(effectiveServerURL, agentName, tag, registryPath, cfg.specPath, tag, readmeContent, string(visibility), cfg.verbose, false, at.Token); err != nil {
		printStepFail()
		return fmt.Errorf("registration failed: %w", err)
	} else {
		printStepDone("")
	}

	agentURL := fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(effectiveServerURL, "/"), at.Account, agentName)

	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	link := lipgloss.NewStyle().Foreground(theme.Primary).Underline(true)

	var lines []string
	lines = append(lines, bold.Render("✓ Pushed successfully!"))
	lines = append(lines, dim.Render("Blueprint is "+string(visibility)))
	lines = append(lines, "")
	lines = append(lines, "  "+bold.Render(agentName)+"  "+dim.Render("tag "+tag))
	lines = append(lines, "  "+dim.Render("View online → ")+link.Render(agentURL))

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Println()
	fmt.Println(box)
	fmt.Println()

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
// If accountOverride is non-empty, it looks up that account and returns its WorkOS org ID.
// Pass "" to use the current logged-in account (the default for push).
func getUserNamespace(verbose bool, accountOverride string) (namespace, workosOrgID string, err error) {
	storage := auth.NewStorage(binaryName)
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		return "", "", fmt.Errorf("not logged in. Run '%s login' to authenticate", binaryName)
	}

	if accountOverride != "" {
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
		var names []string
		for _, acct := range profile.Accounts {
			names = append(names, acct.Name)
		}
		return "", "", fmt.Errorf("account %q not found. Available accounts: %s. Run '%s login' to refresh", accountOverride, strings.Join(names, ", "), binaryName)
	}

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

	if resp.StatusCode == http.StatusUnauthorized && !skipAuth && tokenOverride == "" {
		// Token may have expired mid-push — force refresh and retry once
		resp.Body.Close() //nolint:errcheck,gosec
		retryReq, retryErr := http.NewRequestWithContext(context.Background(), http.MethodPost, reqURL, bytes.NewBuffer(jsonData))
		if retryErr == nil {
			retryReq.Header.Set("Content-Type", "application/json")
			retryReq.Header.Set("X-Cli-Version", version)
			if refreshErr := auth.RefreshAndUpdateHeader(context.Background(), retryReq, binaryName); refreshErr == nil {
				if retryResp, doErr := client.Do(retryReq); doErr == nil { //nolint:gosec
					resp = retryResp
					defer resp.Body.Close() //nolint:errcheck,gosec
				}
			}
		}
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

// confirmVisibilityChange asks the user to confirm the target visibility.
// current may be empty when the agent does not yet exist on the server.
func confirmVisibilityChange(current, desired string) bool {
	var title, description string
	if desired == string(VisibilityPublic) {
		title = "Make blueprint public?"
		description = "This will make the blueprint available to everyone."
		if current == string(VisibilityPrivate) {
			description = "This will change the blueprint from private to public, making it available to everyone."
		}
	} else {
		title = "Make blueprint private?"
		description = fmt.Sprintf("This will change the blueprint from %s to private.", current)
	}

	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
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

// transformSpecForRegistry replaces build sections with actual image references
func transformSpecForRegistry(specObj map[string]interface{}, registry, agentName, tag string) map[string]interface{} {
	// Use the resolved agentName (override or stripped spec name) so the stored
	// spec is consistent with the registration URL and registry image references.
	if _, ok := specObj["name"].(string); ok {
		specObj["name"] = agentName
	}

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
	if tools, ok := specObj["integrations"].(map[string]interface{}); ok {
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
	for _, section := range []string{"models", "knowledge", "integrations", "ingestion"} {
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
