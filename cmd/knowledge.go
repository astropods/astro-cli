package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/astropods/astro-cli/internal/buildinfo"
)

// knowledgeServerURLOverride is set in tests to redirect API calls to a test server.
var knowledgeServerURLOverride string

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage knowledge stores",
	Long:  `Connect, inspect, and delete knowledge stores.`,
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var knowledgeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List knowledge stores",
	Args:  cobra.NoArgs,
	RunE:  runKnowledgeList,
}

var knowledgeStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Get status of a knowledge store",
	Args:  cobra.ExactArgs(1),
	RunE:  runKnowledgeStatus,
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
	Short: "Connect a knowledge store",
	Long:  `Onboard an existing database by providing connection details. The platform stores credentials encrypted under an ARN.`,
	Args:  cobra.NoArgs,
	RunE:  runKnowledgeConnect,
}

func init() {
	rootCmd.AddCommand(knowledgeCmd)
	knowledgeCmd.AddCommand(knowledgeConnectCmd)
	knowledgeCmd.AddCommand(knowledgeListCmd)
	knowledgeCmd.AddCommand(knowledgeStatusCmd)
	knowledgeCmd.AddCommand(knowledgeCredentialsCmd)
	knowledgeCmd.AddCommand(knowledgeDeleteCmd)

	for _, c := range []*cobra.Command{
		knowledgeListCmd, knowledgeStatusCmd, knowledgeCredentialsCmd,
	} {
		c.Flags().Bool("json", false, "Output as JSON")
	}

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

func knowledgeBaseURL() string {
	if knowledgeServerURLOverride != "" {
		return strings.TrimSuffix(knowledgeServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// knowledgePath builds a full URL for a named store. Extra parts become sub-resource segments.
func knowledgePath(account, name string, parts ...string) string {
	segs := append([]string{"knowledge", url.PathEscape(name)}, parts...)
	return apiPath(knowledgeBaseURL(), account, "accounts", segs...)
}

// --- handlers ---

func runKnowledgeConnect(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
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

	reqBody := map[string]any{
		"name":     name,
		"provider": provider,
		"host":     host,
		"port":     port,
	}
	if database != "" {
		reqBody["database"] = database
	}
	if username != "" {
		reqBody["username"] = username
	}
	if password != "" {
		reqBody["password"] = password
	}
	if apiKey != "" {
		reqBody["api_key"] = apiKey
	}
	skipHealthCheck, _ := cmd.Flags().GetBool("skip-health-check")
	if skipHealthCheck {
		reqBody["skip_health_check"] = true
	}
	privateLink, _ := cmd.Flags().GetBool("private-link")
	if privateLink {
		reqBody["private_link"] = true
	}

	var created knowledgeStoreResponse
	status, err := apiCall(cmd.Context(), http.MethodPost,
		apiPath(knowledgeBaseURL(), at.Account, "accounts", "knowledge", "connect"),
		reqBody, at.Token, verbose, &created)
	if err != nil {
		if status == http.StatusConflict {
			return fmt.Errorf("a knowledge store named %q already exists in account %q", name, at.Account)
		}
		return err
	}

	fmt.Printf("  %sARN:%s      %s\n", colorDim, colorReset, created.ARN)
	fmt.Printf("  %sProvider:%s %s\n", colorDim, colorReset, created.Provider)
	fmt.Printf("  %sMode:%s     %s\n", colorDim, colorReset, created.Mode)

	// If PrivateLink was requested, poll until the endpoint is ready.
	if privateLink {
		fmt.Println()
		if err := pollKnowledgePrivateLink(cmd.Context(), at.Account, name, at.Token); err != nil {
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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	jsonOut := flagBool(cmd, "json")

	var stores []knowledgeStoreResponse
	cursor := ""
	for {
		u := apiPath(knowledgeBaseURL(), at.Account, "accounts", "knowledge")
		if cursor != "" {
			u += "?cursor=" + url.QueryEscape(cursor)
		}
		var page knowledgeListResponse
		if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &page); err != nil {
			return err
		}
		stores = append(stores, page.Stores...)
		if page.Page.NextCursor == "" {
			break
		}
		cursor = page.Page.NextCursor
	}

	out := cmd.OutOrStdout()
	if jsonOut {
		return writeJSON(out, stores)
	}

	if len(stores) == 0 {
		fmt.Fprintf(out, "%sNo knowledge stores found.%s\n", colorDim, colorReset) //nolint:errcheck,gosec
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPROVIDER\tSTATUS\tARN")
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
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Provider, statusStr, s.ARN)
	}
	return w.Flush()
}

func runKnowledgeStatus(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	jsonOut := flagBool(cmd, "json")

	var s knowledgeStoreResponse
	status, err := apiCall(cmd.Context(), http.MethodGet,
		knowledgePath(at.Account, args[0]),
		nil, at.Token, verbose, &s)
	if err != nil {
		if status == http.StatusNotFound {
			return fmt.Errorf("knowledge store %q not found", args[0])
		}
		return err
	}

	if jsonOut {
		return writeJSON(os.Stdout, s)
	}

	statusColor := colorReset
	switch s.Status {
	case "ready":
		statusColor = colorGreen
	case "error":
		statusColor = colorRed
	case "connecting", "pending-acceptance":
		statusColor = colorYellow
	}

	fmt.Printf("%sName:%s     %s\n", colorDim, colorReset, s.Name)
	fmt.Printf("%sARN:%s      %s\n", colorDim, colorReset, s.ARN)
	fmt.Printf("%sProvider:%s %s\n", colorDim, colorReset, s.Provider)
	fmt.Printf("%sStatus:%s   %s%s%s\n", colorDim, colorReset, statusColor, s.Status, colorReset)
	if s.Error != nil && *s.Error != "" {
		fmt.Printf("%sError:%s    %s%s%s\n", colorDim, colorReset, colorRed, *s.Error, colorReset)
	}
	fmt.Printf("%sCreated:%s  %s\n", colorDim, colorReset, s.CreatedAt.Format("2006-01-02 15:04:05 UTC"))

	return nil
}

func runKnowledgeCredentials(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	jsonOut := flagBool(cmd, "json")

	var creds map[string]string
	status, err := apiCall(cmd.Context(), http.MethodGet,
		knowledgePath(at.Account, args[0], "credentials"),
		nil, at.Token, verbose, &creds)
	if err != nil {
		if status == http.StatusNotFound {
			return fmt.Errorf("credentials not available for %q (store not found, or KMS was not configured at creation)", args[0])
		}
		return err
	}

	if jsonOut {
		return writeJSON(os.Stdout, creds)
	}

	for k, v := range creds {
		fmt.Printf("%s%s%s=%s\n", colorBold, k, colorReset, v)
	}
	return nil
}

func runKnowledgeDelete(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	name := args[0]
	fmt.Printf("Delete knowledge store %s%s%s in account %s%s%s? [y/N] ", colorBold, name, colorReset, colorBold, at.Account, colorReset)

	var confirm string
	_, _ = fmt.Scanln(&confirm)
	if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
		fmt.Printf("%sCancelled.%s\n", colorDim, colorReset)
		return nil
	}

	status, err := apiCall(cmd.Context(), http.MethodDelete,
		knowledgePath(at.Account, name),
		nil, at.Token, verbose, nil)
	if err != nil {
		if status == http.StatusNotFound {
			return fmt.Errorf("knowledge store %q not found", name)
		}
		if status == http.StatusConflict {
			return fmt.Errorf("knowledge store %q has active agent bindings and cannot be deleted", name)
		}
		return err
	}

	fmt.Printf("%s✓%s Deleted %s%s%s\n", colorGreen, colorReset, colorBold, name, colorReset)
	return nil
}

// pollKnowledgePrivateLink polls the store status during PrivateLink attachment.
// It detects the pending-acceptance state and prints an action-required message.
func pollKnowledgePrivateLink(ctx context.Context, account, name, token string) error {
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

		var s knowledgeStoreResponse
		_, _ = apiCall(ctx, http.MethodGet, knowledgePath(account, name), nil, token, false, &s)

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

type knowledgeStoreEndpointResponse struct {
	CloudProvider   string  `json:"cloud_provider"`
	EndpointService string  `json:"endpoint_service"`
	Region          string  `json:"region"`
	EndpointID      *string `json:"endpoint_id,omitempty"`
	EndpointDNS     *string `json:"endpoint_dns,omitempty"`
	Status          string  `json:"status"`
	Error           *string `json:"error,omitempty"`
}

// knowledgeStoreResponse mirrors the server's knowledge response shape.
// knowledgeListResponse is the account list's page envelope. The server moved
// from a bare array to this shape when /me/knowledge was retired; NextCursor
// is empty on the last page.
type knowledgeListResponse struct {
	Stores []knowledgeStoreResponse `json:"stores"`
	Page   struct {
		Limit      int    `json:"limit"`
		NextCursor string `json:"next_cursor,omitempty"`
	} `json:"page"`
}

type knowledgeStoreResponse struct {
	ID        string                          `json:"id"`
	ARN       string                          `json:"arn"`
	Name      string                          `json:"name"`
	Provider  string                          `json:"provider"`
	Mode      string                          `json:"mode"`
	Status    string                          `json:"status"`
	Endpoint  *knowledgeStoreEndpointResponse `json:"endpoint,omitempty"`
	Error     *string                         `json:"error,omitempty"`
	CreatedAt time.Time                       `json:"created_at"`
}
