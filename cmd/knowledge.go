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
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
)

// knowledgeServerURL mirrors the pattern from push.go — overridable per-command.
var knowledgeServerURL string
var knowledgeAccount string
var knowledgeOutput string // -o / --output: "" (default) or "json"

func displayMode(mode string) string {
	if mode == "" {
		return "managed"
	}
	return mode
}

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage managed knowledge stores",
	Long:  `Create, inspect, and delete managed knowledge stores.`,
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var knowledgeCreateCmd = &cobra.Command{
	Use:   "create --provider <provider> --name <name>",
	Short: "Create a managed knowledge store",
	Args:  cobra.NoArgs,
	RunE:  runKnowledgeCreate,
}

var knowledgeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed knowledge stores",
	Args:  cobra.NoArgs,
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

var knowledgeConnectCmd = &cobra.Command{
	Use:   "connect --provider <provider> --name <name> --host <host> --port <port>",
	Short: "Connect an external knowledge store",
	Long:  `Onboard an existing database by providing connection details. The platform stores credentials encrypted under an ARN.`,
	Args:  cobra.NoArgs,
	RunE:  runKnowledgeConnect,
}

func init() {
	rootCmd.AddCommand(knowledgeCmd)
	knowledgeCmd.AddCommand(knowledgeCreateCmd)
	knowledgeCmd.AddCommand(knowledgeConnectCmd)
	knowledgeCmd.AddCommand(knowledgeListCmd)
	knowledgeCmd.AddCommand(knowledgeStatusCmd)
	knowledgeCmd.AddCommand(knowledgeLogsCmd)
	knowledgeCmd.AddCommand(knowledgeCredentialsCmd)
	knowledgeCmd.AddCommand(knowledgeDeleteCmd)

	for _, c := range []*cobra.Command{
		knowledgeCreateCmd, knowledgeConnectCmd, knowledgeListCmd, knowledgeStatusCmd,
		knowledgeLogsCmd, knowledgeCredentialsCmd, knowledgeDeleteCmd,
	} {
		c.Flags().StringVar(&knowledgeServerURL, "server", "", "Astropods server URL (overrides profile/default)")
		c.Flags().StringVar(&knowledgeAccount, "account", "", "Account name (overrides profile default)")
	}

	for _, c := range []*cobra.Command{
		knowledgeListCmd, knowledgeStatusCmd, knowledgeCredentialsCmd,
	} {
		c.Flags().StringVarP(&knowledgeOutput, "output", "o", "", "Output format: json")
	}

	knowledgeLogsCmd.Flags().Bool("tail", false, "Follow logs in real time")

	knowledgeCreateCmd.Flags().String("provider", "", "Database provider: postgres, qdrant, redis, neo4j")
	knowledgeCreateCmd.Flags().String("name", "", "Store name")
	knowledgeCreateCmd.Flags().String("storage", "10Gi", "Storage size (e.g. 20Gi)")
	knowledgeCreateCmd.Flags().String("storage-class", "", "Kubernetes StorageClass name (default: cluster default)")
	knowledgeCreateCmd.Flags().Bool("public", false, "Expose the store publicly with a DNS hostname")
	_ = knowledgeCreateCmd.MarkFlagRequired("provider")
	_ = knowledgeCreateCmd.MarkFlagRequired("name")

	knowledgeConnectCmd.Flags().String("provider", "", "Database provider: postgres, qdrant, redis, neo4j, pinecone")
	knowledgeConnectCmd.Flags().String("name", "", "Store name")
	knowledgeConnectCmd.Flags().String("host", "", "Database host")
	knowledgeConnectCmd.Flags().Int("port", 0, "Database port")
	knowledgeConnectCmd.Flags().String("database", "", "Database name (if applicable)")
	knowledgeConnectCmd.Flags().String("username", "", "Database username")
	knowledgeConnectCmd.Flags().String("password", "", "Database password (prompted if omitted)")
	knowledgeConnectCmd.Flags().String("api-key", "", "API key (for providers like Pinecone/Qdrant)")
	knowledgeConnectCmd.Flags().Bool("skip-health-check", false, "Skip connectivity check (use when the store is behind PrivateLink or a firewall)")
	knowledgeConnectCmd.Flags().Bool("private-link", false, "Connect via AWS PrivateLink (host must be a VPC endpoint service name)")
	_ = knowledgeConnectCmd.MarkFlagRequired("provider")
	_ = knowledgeConnectCmd.MarkFlagRequired("name")
	_ = knowledgeConnectCmd.MarkFlagRequired("host")
	_ = knowledgeConnectCmd.MarkFlagRequired("port")

}

// knowledgeAPIBase returns the effective server URL for knowledge API calls.
func knowledgeAPIBase() string {
	if knowledgeServerURL != "" {
		return strings.TrimSuffix(knowledgeServerURL, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
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
	req.Header.Set("X-Cli-Version", buildinfo.Version)
	if err := auth.AddAuthHeader(ctx, req, buildinfo.BinaryName); err != nil {
		return nil, fmt.Errorf("authentication failed: %w. Run '%s login' to re-authenticate", err, buildinfo.BinaryName)
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
	storageClass, _ := cmd.Flags().GetString("storage-class")
	public, _ := cmd.Flags().GetBool("public")

	fmt.Printf("%s→%s Creating knowledge store %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset)

	body := map[string]any{
		"name":     name,
		"provider": provider,
		"storage":  storage,
		"public":   public,
	}
	if storageClass != "" {
		body["storage_class"] = storageClass
	}

	resp, err := knowledgeRequest(cmd.Context(), http.MethodPost,
		fmt.Sprintf("/api/v1/accounts/%s/knowledge", url.PathEscape(account)),
		body,
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

	if err := pollKnowledgeReady(cmd.Context(), account, created.Name); err != nil {
		fmt.Println()
		return err
	}

	fmt.Printf("\n%s✓%s Store %s%s%s is ready\n", colorGreen, colorReset, colorBold, name, colorReset)
	return nil
}

func runKnowledgeConnect(cmd *cobra.Command, _ []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	name, _ := cmd.Flags().GetString("name")
	provider, _ := cmd.Flags().GetString("provider")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	database, _ := cmd.Flags().GetString("database")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	apiKey, _ := cmd.Flags().GetString("api-key")

	// Prompt for password interactively if not provided via flag.
	if password == "" && apiKey == "" {
		fmt.Print("Password: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println()
		password = string(raw)
	}

	fmt.Printf("%s→%s Connecting external store %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset)

	body := map[string]any{
		"name":     name,
		"provider": provider,
		"host":     host,
		"port":     port,
	}
	if database != "" {
		body["database"] = database
	}
	if username != "" {
		body["username"] = username
	}
	if password != "" {
		body["password"] = password
	}
	if apiKey != "" {
		body["api_key"] = apiKey
	}
	skipHealthCheck, _ := cmd.Flags().GetBool("skip-health-check")
	if skipHealthCheck {
		body["skip_health_check"] = true
	}
	privateLink, _ := cmd.Flags().GetBool("private-link")
	if privateLink {
		body["private_link"] = true
	}

	resp, err := knowledgeRequest(cmd.Context(), http.MethodPost,
		fmt.Sprintf("/api/v1/accounts/%s/knowledge/connect", url.PathEscape(account)),
		body,
	)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("a knowledge store named %q already exists in account %q", name, account)
	}
	if resp.StatusCode != http.StatusOK {
		var respBody map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&respBody)
		if msg, ok := respBody["error"].(string); ok {
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
	fmt.Printf("  %sMode:%s     %s\n", colorDim, colorReset, created.Mode)

	// If PrivateLink was requested, poll until the endpoint is ready.
	if privateLink {
		fmt.Println()
		if err := pollKnowledgePrivateLink(cmd.Context(), account, name); err != nil {
			fmt.Println()
			return err
		}
		return nil
	}

	if created.Status == "error" && created.Error != nil && *created.Error != "" {
		fmt.Printf("%s⚠%s Store %s%s%s connected but %s\n", colorYellow, colorReset, colorBold, name, colorReset, *created.Error)
		fmt.Printf("  The store was created but is not reachable. Fix connectivity and reconnect, or use --skip-health-check.\n")
	} else {
		fmt.Printf("%s✓%s Store %s%s%s connected\n", colorGreen, colorReset, colorBold, name, colorReset)
	}
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

	if knowledgeOutput == "json" {
		return json.NewEncoder(os.Stdout).Encode(stores)
	}

	if len(stores) == 0 {
		fmt.Printf("%sNo knowledge stores found.%s\n", colorDim, colorReset)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPROVIDER\tMODE\tSTATUS\tSTORAGE\tARN")
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
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, s.Provider, displayMode(s.Mode), statusStr, s.Storage, s.ARN)
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

	if knowledgeOutput == "json" {
		return json.NewEncoder(os.Stdout).Encode(s)
	}

	statusColor := colorReset
	switch s.Status {
	case "ready":
		statusColor = colorGreen
	case "error":
		statusColor = colorRed
	case "provisioning", "connecting", "pending-acceptance":
		statusColor = colorYellow
	}

	fmt.Printf("%sName:%s     %s\n", colorDim, colorReset, s.Name)
	fmt.Printf("%sARN:%s      %s\n", colorDim, colorReset, s.ARN)
	fmt.Printf("%sProvider:%s %s\n", colorDim, colorReset, s.Provider)
	fmt.Printf("%sMode:%s     %s\n", colorDim, colorReset, displayMode(s.Mode))
	fmt.Printf("%sStatus:%s   %s%s%s\n", colorDim, colorReset, statusColor, s.Status, colorReset)
	fmt.Printf("%sStorage:%s  %s\n", colorDim, colorReset, s.Storage)
	if s.PublicHost != nil && *s.PublicHost != "" {
		fmt.Printf("%sHost:%s     %s\n", colorDim, colorReset, *s.PublicHost)
	}
	if s.Error != nil && *s.Error != "" {
		fmt.Printf("%sError:%s    %s%s%s\n", colorDim, colorReset, colorRed, *s.Error, colorReset)
	}
	fmt.Printf("%sCreated:%s  %s\n", colorDim, colorReset, s.CreatedAt.Format("2006-01-02 15:04:05 UTC"))

	if len(s.Events) > 0 {
		e := s.Events[0]
		icon := "·"
		if e.Type == "Warning" {
			icon = colorRed + "!" + colorReset
		}
		countStr := ""
		if e.Count > 1 {
			countStr = fmt.Sprintf(" %s(×%d)%s", colorDim, e.Count, colorReset)
		}
		fmt.Printf("%sEvent:%s    %s %s%s:%s %s%s\n", colorDim, colorReset, icon, colorDim, e.Reason, colorReset, e.Message, countStr)
	}

	return nil
}

func runKnowledgeLogs(cmd *cobra.Command, args []string) error {
	account, _, err := getUserNamespace(false, knowledgeAccount)
	if err != nil {
		return err
	}

	tail, _ := cmd.Flags().GetBool("tail")
	if tail {
		return runKnowledgeLogsTail(cmd.Context(), account, args[0])
	}

	// Historical logs — one-shot JSON fetch.
	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet,
		knowledgeAPIBase()+knowledgePath(account, args[0])+"/logs", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Cli-Version", buildinfo.Version)
	if err := auth.AddAuthHeader(cmd.Context(), req, buildinfo.BinaryName); err != nil {
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

	return printLogs(cmd.OutOrStdout(), resp.Body)
}

func runKnowledgeLogsTail(parentCtx context.Context, account, name string) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	url := knowledgeAPIBase() + knowledgePath(account, name) + "/logs/stream"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Cli-Version", buildinfo.Version)
	if err := auth.AddAuthHeader(ctx, req, buildinfo.BinaryName); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE fields: "data: ...", "event: ...", "id: ...", or blank line.
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var entry logEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			continue // skip non-log events (ready, status, heartbeat, error)
		}
		if entry.Message == "" {
			continue
		}

		level := entry.Level
		if level == "" {
			level = "INFO"
		}
		if entry.Timestamp != "" {
			fmt.Printf("%s %s %s\n", entry.Timestamp, level, entry.Message)
		} else {
			fmt.Printf("%s %s\n", level, entry.Message)
		}
	}

	if ctx.Err() != nil {
		return nil
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

	if knowledgeOutput == "json" {
		return json.NewEncoder(os.Stdout).Encode(creds)
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
		fmt.Printf("%sCancelled.%s\n", colorDim, colorReset)
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

// pollKnowledgePrivateLink polls the store status during PrivateLink attachment.
// It detects the pending-acceptance state and prints an action-required message.
func pollKnowledgePrivateLink(ctx context.Context, account, name string) error {
	const (
		pollInterval = 3 * time.Second
		timeout      = 15 * time.Minute
	)
	deadline := time.Now().Add(timeout)
	printedAcceptance := false

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		resp, err := knowledgeRequest(ctx, http.MethodGet, knowledgePath(account, name), nil)
		if err != nil {
			fmt.Print(".")
			continue
		}
		var s knowledgeStoreResponse
		_ = json.NewDecoder(resp.Body).Decode(&s)
		_ = resp.Body.Close()

		switch s.Status {
		case "connecting":
			fmt.Printf("\r%sStatus:%s connecting", colorDim, colorReset)
		case "pending-acceptance":
			if !printedAcceptance {
				printedAcceptance = true
				fmt.Printf("\r%sStatus:%s %spending-acceptance%s\n", colorDim, colorReset, colorYellow, colorReset)
				fmt.Println()
				fmt.Printf("  %sAction required:%s accept the endpoint connection request in your AWS console.\n", colorBold, colorReset)
				if s.Endpoint != nil {
					fmt.Printf("  %sEndpoint service:%s %s\n", colorDim, colorReset, s.Endpoint.EndpointService)
					if s.Endpoint.EndpointID != nil {
						fmt.Printf("  %sVPC endpoint:%s     %s\n", colorDim, colorReset, *s.Endpoint.EndpointID)
					}
				}
				fmt.Println()
				fmt.Printf("  Waiting for acceptance")
			} else {
				fmt.Print(".")
			}
		case "ready":
			dns := ""
			if s.Endpoint != nil && s.Endpoint.EndpointDNS != nil {
				dns = *s.Endpoint.EndpointDNS
			}
			fmt.Printf("\n%s✓%s PrivateLink ready", colorGreen, colorReset)
			if dns != "" {
				fmt.Printf(" — endpoint: %s", dns)
			}
			fmt.Println()
			return nil
		case "error":
			if s.Error != nil && *s.Error != "" {
				return fmt.Errorf("PrivateLink failed: %s", *s.Error)
			}
			return fmt.Errorf("PrivateLink failed")
		}
	}
	return fmt.Errorf("timed out waiting for PrivateLink to become ready")
}

// pollKnowledgeReady polls the status endpoint every 3 seconds until the store
// reaches a terminal state. Avoids long-lived SSE connections that proxies drop.
func pollKnowledgeReady(ctx context.Context, account, name string) error {
	const (
		pollInterval = 3 * time.Second
		timeout      = 15 * time.Minute
	)
	deadline := time.Now().Add(timeout)
	lastEvent := ""

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		resp, err := knowledgeRequest(ctx, http.MethodGet, knowledgePath(account, name), nil)
		if err != nil {
			fmt.Print(".")
			continue
		}
		var s knowledgeStoreResponse
		_ = json.NewDecoder(resp.Body).Decode(&s)
		_ = resp.Body.Close()

		if len(s.Events) > 0 {
			e := s.Events[0]
			key := e.Reason + ":" + e.Message
			if key != lastEvent {
				lastEvent = key
				fmt.Printf("\n  %s%s:%s %s", colorDim, e.Reason, colorReset, e.Message)
			}
		} else {
			fmt.Print(".")
		}

		switch s.Status {
		case "ready":
			return nil
		case "error":
			if s.Error != nil && *s.Error != "" {
				return fmt.Errorf("provisioning failed: %s", *s.Error)
			}
			return fmt.Errorf("provisioning failed")
		}
	}
	return fmt.Errorf("timed out waiting for store to become ready")
}

// knowledgeStoreResponse mirrors the server's knowledge response shape.
type knowledgeStoreEvent struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Count   int32  `json:"count"`
}

type knowledgeStoreEndpointResponse struct {
	CloudProvider   string  `json:"cloud_provider"`
	EndpointService string  `json:"endpoint_service"`
	Region          string  `json:"region"`
	EndpointID      *string `json:"endpoint_id,omitempty"`
	EndpointDNS     *string `json:"endpoint_dns,omitempty"`
	Status          string  `json:"status"`
	Error           *string `json:"error,omitempty"`
}

type knowledgeStoreResponse struct {
	ID         string                          `json:"id"`
	ARN        string                          `json:"arn"`
	Name       string                          `json:"name"`
	Provider   string                          `json:"provider"`
	Mode       string                          `json:"mode"`
	Status     string                          `json:"status"`
	Storage    string                          `json:"storage"`
	Public     bool                            `json:"public"`
	PublicHost *string                         `json:"public_host,omitempty"`
	Endpoint   *knowledgeStoreEndpointResponse `json:"endpoint,omitempty"`
	Error      *string                         `json:"error,omitempty"`
	Events     []knowledgeStoreEvent           `json:"events,omitempty"`
	CreatedAt  time.Time                       `json:"created_at"`
}
