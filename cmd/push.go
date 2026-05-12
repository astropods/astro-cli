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
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// pushServerURLOverride is set in tests to redirect API calls to a test server.
var pushServerURLOverride string

func pushBaseURL() string {
	if pushServerURLOverride != "" {
		return strings.TrimSuffix(pushServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

func pushRegistryURL() string {
	if pushServerURLOverride != "" {
		return auth.RegistryURLFromServerURL(pushServerURLOverride)
	}
	if buildinfo.DefaultRegistryURL != "" {
		return strings.TrimSuffix(buildinfo.DefaultRegistryURL, "/")
	}
	return auth.RegistryURLFromServerURL(buildinfo.DefaultServerURL)
}

// runPush assumes the spec in cfg.SpecPath is valid; callers must validate before invoking.
func runPush(ctx context.Context, at AccountToken, cfg PushPipelineConfig) error {
	serverURL := pushBaseURL()
	registryURL := pushRegistryURL()

	if cfg.Verbose {
		fmt.Printf("%s→%s Server URL:   %s%s%s\n", colorCyan, colorReset, colorDim, serverURL, colorReset)
		fmt.Printf("%s→%s Registry URL: %s%s%s\n", colorCyan, colorReset, colorDim, registryURL, colorReset)
	}

	if registryURL == "" {
		return fmt.Errorf("registry URL required: run '%s login'", buildinfo.BinaryName)
	}
	if serverURL == "" {
		return fmt.Errorf("server URL required: run '%s login'", buildinfo.BinaryName)
	}

	registryHost, err := getRegistryHost(registryURL)
	if err != nil {
		return fmt.Errorf("failed to parse registry URL: %w", err)
	}

	// Fill in fields resolved at push time
	cfg.RegistryHost = registryHost
	cfg.Account = at.Account
	cfg.Token = at.Token

	pipeline := NewPushPipeline(ctx, cfg)

	fmt.Printf("%s→%s Pushing %s%s%s to %s%s%s build %s\n\n",
		colorCyan, colorReset, colorBold, cfg.AgentName, colorReset,
		colorCyan, at.Account, colorReset, pipeline.Tag())

	if err := pipeline.
		ParseSpec().
		CollectComponents().
		Build().
		Push().
		TransformSpec().
		StripSecrets().
		LoadReadme().
		ResolveVisibility().
		Register().
		Err(); err != nil {
		return err
	}

	pipeline.PrintSuccess()
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

// registerAgent registers the agent spec with the astro-server.
// It reads the spec from disk, transforms build→image, strips secrets, and POSTs.
// Kept for backward compatibility with existing tests; new code should use the PushPipeline.
func registerAgent(serverURL, agentName, buildID, registry, specPath, pushTag, readme, visibility string, verbose bool, skipAuth bool, tokenOverride string) error {
	specData, err := os.ReadFile(specPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}
	var specObj map[string]any
	if err := yaml.Unmarshal(specData, &specObj); err != nil {
		return fmt.Errorf("failed to parse spec YAML: %w", err)
	}
	spec.TransformSpecForRegistry(specObj, agentName, func(imageName string) string {
		return fmt.Sprintf("%s/%s:%s", registry, imageName, pushTag)
	})
	spec.StripSecretDefaults(specObj)
	transformedSpecData, err := yaml.Marshal(specObj)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed spec: %w", err)
	}
	return registerAgentWithServer(serverURL, agentName, buildID, registry, string(transformedSpecData), readme, visibility, verbose, skipAuth, tokenOverride)
}

// registerAgentWithServer sends the already-transformed spec content to the server.
func registerAgentWithServer(serverURL, agentName, buildID, registry, specContent, readme, visibility string, verbose bool, skipAuth bool, tokenOverride string) error {
	// Extract account name from registry path (registryHost/accountName)
	accountName := ""
	registryParts := strings.Split(registry, "/")
	if len(registryParts) >= 2 {
		accountName = registryParts[len(registryParts)-1]
	}

	// Prepare request payload
	payload := map[string]string{
		"build_id":     buildID,
		"registry":     registry,
		"spec_content": specContent,
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
	req.Header.Set("X-Cli-Version", buildinfo.Version)

	// Add authentication header if not skipped
	if !skipAuth {
		if tokenOverride != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenOverride))
		} else if err := auth.AddAuthHeader(context.Background(), req, buildinfo.BinaryName); err != nil {
			return fmt.Errorf("failed to add authentication: %w. Run '%s login' to re-authenticate", err, buildinfo.BinaryName)
		}
	}
	if verbose {
		authHeader := req.Header.Get("Authorization")
		if authHeader != "" {
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
				return fmt.Errorf("%s\nRun '%s upgrade' to update", msg, buildinfo.BinaryName)
			}
		}
		return fmt.Errorf("CLI version %s is too old. Run '%s upgrade' to update", buildinfo.Version, buildinfo.BinaryName)
	}

	if resp.StatusCode == http.StatusUnauthorized && !skipAuth && tokenOverride == "" {
		// Token may have expired mid-push — force refresh and retry once
		resp.Body.Close() //nolint:errcheck,gosec
		retryReq, retryErr := http.NewRequestWithContext(context.Background(), http.MethodPost, reqURL, bytes.NewBuffer(jsonData))
		if retryErr == nil {
			retryReq.Header.Set("Content-Type", "application/json")
			retryReq.Header.Set("X-Cli-Version", buildinfo.Version)
			if refreshErr := auth.RefreshAndUpdateHeader(context.Background(), retryReq, buildinfo.BinaryName); refreshErr == nil {
				if retryResp, doErr := client.Do(retryReq); doErr == nil { //nolint:gosec
					resp = retryResp
					defer resp.Body.Close() //nolint:errcheck,gosec
				}
			}
		}
	}

	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed (401). Server response: %s\nRun '%s login' to re-authenticate", string(body), buildinfo.BinaryName)
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
		} else if err := auth.AddAuthHeader(context.Background(), req, buildinfo.BinaryName); err != nil {
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

	form.WithTheme(cliHuhTheme())

	if err := form.Run(); err != nil {
		return false
	}

	return confirmed
}
