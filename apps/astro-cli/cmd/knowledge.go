package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
)

// knowledgeServerURL mirrors the pattern from push.go — overridable per-command.
var knowledgeServerURL string
var knowledgeAccount string

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage managed knowledge stores",
	Long:  `Create, inspect, and delete managed knowledge stores.`,
}

var knowledgeCreateCmd = &cobra.Command{
	Use:   "create --provider <provider> --name <name>",
	Short: "Create a managed knowledge store",
	RunE:  runKnowledgeCreate,
}

var knowledgeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed knowledge stores",
	RunE:  runKnowledgeList,
}

var knowledgeStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Get status of a knowledge store",
	Args:  cobra.ExactArgs(1),
	RunE:  runKnowledgeStatus,
}

var knowledgeLogsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Stream container logs from a knowledge store",
	Args:  cobra.ExactArgs(1),
	RunE:  runKnowledgeLogs,
}

var knowledgeCredentialsCmd = &cobra.Command{
	Use:   "credentials <name>",
	Short: "Print credentials for a knowledge store",
	Args:  cobra.ExactArgs(1),
	RunE:  runKnowledgeCredentials,
}

var knowledgeDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a knowledge store",
	Args:  cobra.ExactArgs(1),
	RunE:  runKnowledgeDelete,
}

func init() {
	rootCmd.AddCommand(knowledgeCmd)
	knowledgeCmd.AddCommand(knowledgeCreateCmd)
	knowledgeCmd.AddCommand(knowledgeListCmd)
	knowledgeCmd.AddCommand(knowledgeStatusCmd)
	knowledgeCmd.AddCommand(knowledgeLogsCmd)
	knowledgeCmd.AddCommand(knowledgeCredentialsCmd)
	knowledgeCmd.AddCommand(knowledgeDeleteCmd)

	for _, c := range []*cobra.Command{
		knowledgeCreateCmd, knowledgeListCmd, knowledgeStatusCmd,
		knowledgeLogsCmd, knowledgeCredentialsCmd, knowledgeDeleteCmd,
	} {
		c.Flags().StringVar(&knowledgeServerURL, "server", "", "Astro server URL (overrides profile/default)")
		c.Flags().StringVar(&knowledgeAccount, "account", "", "Account name (overrides profile default)")
	}

	knowledgeCreateCmd.Flags().String("provider", "", "Database provider: postgres, qdrant, redis, neo4j")
	knowledgeCreateCmd.Flags().String("name", "", "Store name")
	knowledgeCreateCmd.Flags().String("storage", "10Gi", "Storage size (e.g. 20Gi)")
	knowledgeCreateCmd.Flags().Bool("public", false, "Expose the store publicly with a DNS hostname")
	_ = knowledgeCreateCmd.MarkFlagRequired("provider")
	_ = knowledgeCreateCmd.MarkFlagRequired("name")
}

// knowledgeAPIBase returns the effective server URL for knowledge API calls.
func knowledgeAPIBase() string {
	if knowledgeServerURL != "" {
		return strings.TrimSuffix(knowledgeServerURL, "/")
	}
	return strings.TrimSuffix(auth.DefaultServerURL, "/")
}

func knowledgeRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(b)
	} else {
		bodyReader = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, knowledgeAPIBase()+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cli-Version", version)
	if err := auth.AddAuthHeader(ctx, req, binaryName); err != nil {
		return nil, fmt.Errorf("authentication failed: %w. Run '%s login' to re-authenticate", err, binaryName)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req) //nolint:gosec
}

func knowledgePath(account, name string) string {
	return fmt.Sprintf("/api/v1/accounts/%s/knowledge/%s", url.PathEscape(account), url.PathEscape(name))
}

// --- handlers ---

func runKnowledgeCreate(cmd *cobra.Command, _ []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	provider, _ := cmd.Flags().GetString("provider")
	storage, _ := cmd.Flags().GetString("storage")
	public, _ := cmd.Flags().GetBool("public")

	fmt.Printf("%s→%s Creating knowledge store %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset)

	resp, err := knowledgeRequest(cmd.Context(), http.MethodPost,
		fmt.Sprintf("/api/v1/accounts/%s/knowledge", url.PathEscape(account)),
		map[string]any{
			"name":     name,
			"provider": provider,
			"storage":  storage,
			"public":   public,
		},
	)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("a knowledge store named %q already exists in account %q", name, account)
	}
	if resp.StatusCode != http.StatusAccepted {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if msg, ok := body["error"].(string); ok {
			return fmt.Errorf("server error: %s", msg)
		}
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var created knowledgeStoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("  %sARN:%s      %s\n", colorDim, colorReset, created.ARN)
	fmt.Printf("  %sProvider:%s %s\n", colorDim, colorReset, created.Provider)
	if created.PublicHost != nil && *created.PublicHost != "" {
		fmt.Printf("  %sHost:%s     %s\n", colorDim, colorReset, *created.PublicHost)
	}
	fmt.Println()
	fmt.Printf("%s→%s Provisioning", colorCyan, colorReset)

	// Stream events until the store reaches a terminal state.
	if err := streamKnowledgeEvents(cmd.Context(), account, created.ID); err != nil {
		fmt.Println()
		return err
	}

	fmt.Printf("\n%s✓%s Store %s%s%s is ready\n", colorGreen, colorReset, colorBold, name, colorReset)
	return nil
}

func runKnowledgeList(cmd *cobra.Command, _ []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	resp, err := knowledgeRequest(cmd.Context(), http.MethodGet,
		fmt.Sprintf("/api/v1/accounts/%s/knowledge", url.PathEscape(account)), nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var stores []knowledgeStoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&stores); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(stores) == 0 {
		fmt.Printf("%sNo knowledge stores found.%s\n", colorDim, colorReset)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPROVIDER\tSTATUS\tSTORAGE\tARN")
	for _, s := range stores {
		var statusStr string
		switch s.Status {
		case "ready":
			statusStr = colorGreen + "ready" + colorReset
		case "error":
			statusStr = colorRed + "error" + colorReset
		default:
			statusStr = s.Status
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.Provider, statusStr, s.Storage, s.ARN)
	}
	return w.Flush()
}

func runKnowledgeStatus(cmd *cobra.Command, args []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	resp, err := knowledgeRequest(cmd.Context(), http.MethodGet, knowledgePath(account, args[0]), nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("knowledge store %q not found", args[0])
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var s knowledgeStoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	statusColor := colorReset
	switch s.Status {
	case "ready":
		statusColor = colorGreen
	case "error":
		statusColor = colorRed
	case "provisioning":
		statusColor = colorYellow
	}

	fmt.Printf("%sName:%s     %s\n", colorDim, colorReset, s.Name)
	fmt.Printf("%sARN:%s      %s\n", colorDim, colorReset, s.ARN)
	fmt.Printf("%sProvider:%s %s\n", colorDim, colorReset, s.Provider)
	fmt.Printf("%sStatus:%s   %s%s%s\n", colorDim, colorReset, statusColor, s.Status, colorReset)
	fmt.Printf("%sStorage:%s  %s\n", colorDim, colorReset, s.Storage)
	if s.PublicHost != nil && *s.PublicHost != "" {
		fmt.Printf("%sHost:%s     %s\n", colorDim, colorReset, *s.PublicHost)
	}
	if s.Error != nil && *s.Error != "" {
		fmt.Printf("%sError:%s    %s%s%s\n", colorDim, colorReset, colorRed, *s.Error, colorReset)
	}
	fmt.Printf("%sCreated:%s  %s\n", colorDim, colorReset, s.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
	return nil
}

func runKnowledgeLogs(cmd *cobra.Command, args []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	// Logs endpoint streams plain text — use a long-running client.
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet,
		knowledgeAPIBase()+knowledgePath(account, args[0])+"/logs", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Cli-Version", version)
	if err := auth.AddAuthHeader(cmd.Context(), req, binaryName); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("knowledge store %q not found", args[0])
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	return scanner.Err()
}

func runKnowledgeCredentials(cmd *cobra.Command, args []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	resp, err := knowledgeRequest(cmd.Context(), http.MethodGet,
		knowledgePath(account, args[0])+"/credentials", nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("credentials not available for %q (store not found, or KMS was not configured at creation)", args[0])
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var creds map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&creds); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	for k, v := range creds {
		fmt.Printf("%s%s%s=%s\n", colorBold, k, colorReset, v)
	}
	return nil
}

func runKnowledgeDelete(cmd *cobra.Command, args []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	name := args[0]
	fmt.Printf("Delete knowledge store %s%s%s in account %s%s%s? [y/N] ", colorBold, name, colorReset, colorBold, account, colorReset)

	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		fmt.Println("Aborted.")
		return nil
	}

	resp, err := knowledgeRequest(cmd.Context(), http.MethodDelete, knowledgePath(account, name), nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("knowledge store %q not found", name)
	}
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("knowledge store %q has active agent bindings and cannot be deleted", name)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	fmt.Printf("%s✓%s Deleted %s%s%s\n", colorGreen, colorReset, colorBold, name, colorReset)
	return nil
}

// streamKnowledgeEvents connects to the /events SSE endpoint and prints
// progress until the store reaches a terminal state (ready or error).
func streamKnowledgeEvents(ctx context.Context, account, storeID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		knowledgeAPIBase()+fmt.Sprintf("/api/v1/accounts/%s/knowledge/%s/events",
			url.PathEscape(account), url.PathEscape(storeID)),
		nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Cli-Version", version)
	if err := auth.AddAuthHeader(ctx, req, binaryName); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to connect to event stream: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("events stream returned status %d", resp.StatusCode)
	}

	type statusEvent struct {
		Status  string `json:"status"`
		StoreID string `json:"store_id"`
		Error   string `json:"error"`
		Type    string `json:"type"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}

	scanner := bufio.NewScanner(resp.Body)
	seen := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		var evt statusEvent
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}

		if evt.Error != "" {
			return fmt.Errorf("store error: %s", evt.Error)
		}

		// Deduplicate K8s events by reason+message.
		if evt.Reason != "" {
			key := evt.Reason + ":" + evt.Message
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Printf("\n  %s%s:%s %s", colorDim, evt.Reason, colorReset, evt.Message)
		} else {
			fmt.Print(".")
		}

		if evt.Status == "ready" {
			return nil
		}
		if evt.Status == "error" {
			return fmt.Errorf("provisioning failed")
		}
	}

	return scanner.Err()
}

// knowledgeStoreResponse mirrors the server's knowledge response shape.
type knowledgeStoreResponse struct {
	ID         string    `json:"id"`
	ARN        string    `json:"arn"`
	Name       string    `json:"name"`
	Provider   string    `json:"provider"`
	Status     string    `json:"status"`
	Storage    string    `json:"storage"`
	Public     bool      `json:"public"`
	PublicHost *string   `json:"public_host,omitempty"`
	Error      *string   `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
