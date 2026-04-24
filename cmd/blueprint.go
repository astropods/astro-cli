package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
)

// blueprintServerURLOverride is set in tests to redirect API calls to a test server.
var blueprintServerURLOverride string

var blueprintNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{4,}$`)

func validateBlueprintName(name string) error {
	if !blueprintNameRe.MatchString(name) {
		return fmt.Errorf("blueprint name must be at least 4 characters and contain only letters, digits, underscores, or hyphens")
	}
	return nil
}

// validateVisibility returns an error if v is not "public" or "private".
func validateVisibility(v string) error {
	if v != "public" && v != "private" {
		return fmt.Errorf("--visibility must be 'public' or 'private', got %q", v)
	}
	return nil
}

var blueprintCmd = &cobra.Command{
	Use:     "blueprint",
	Aliases: []string{"bp"},
	Short:   "Manage agent blueprints",
	Long:    "Manage agent blueprints — registered, versioned agent definitions on the platform.",
}

var blueprintListCmd = &cobra.Command{
	Use:   "list",
	Short: "List blueprints in the active account",
	RunE:  runBlueprintList,
}

var blueprintCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Register a new blueprint on the server",
	Args:  cobra.ExactArgs(1),
	RunE:  runBlueprintCreate,
}

var blueprintGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get blueprint metadata and config",
	Args:  cobra.ExactArgs(1),
	RunE:  runBlueprintGet,
}

var blueprintPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push blueprint image to registry",
	Long:  pushCmd.Long + "\n\nIf the blueprint does not yet exist it will be created automatically.",
	RunE:  runBlueprintPush,
}

var blueprintArchiveCmd = &cobra.Command{
	Use:   "archive <name>",
	Short: "Archive a blueprint (soft delete)",
	Args:  cobra.ExactArgs(1),
	RunE:  runBlueprintArchive,
}

var blueprintUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update blueprint settings",
	Args:  cobra.ExactArgs(1),
	RunE:  runBlueprintUpdate,
}

func init() {
	rootCmd.AddCommand(blueprintCmd)
	blueprintCmd.AddCommand(blueprintListCmd)
	blueprintCmd.AddCommand(blueprintCreateCmd)
	blueprintCmd.AddCommand(blueprintGetCmd)
	blueprintCmd.AddCommand(blueprintPushCmd)
	blueprintCmd.AddCommand(blueprintArchiveCmd)
	blueprintCmd.AddCommand(blueprintUpdateCmd)

	blueprintListCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintGetCmd.Flags().Bool("card", false, "Show agent description")

	// Create flags
	blueprintCreateCmd.Flags().StringP("visibility", "V", "private", "Set visibility: public or private")

	// Update flags
	blueprintUpdateCmd.Flags().StringP("visibility", "V", "", "Set visibility: public or private")

	// Push flags
	blueprintPushCmd.Flags().Bool("build", false, "Build image before pushing")
	blueprintPushCmd.Flags().StringP("visibility", "V", "", "Set visibility: public or private")
}

func runBlueprintPush(cmd *cobra.Command, args []string) error {
	build, _ := cmd.Flags().GetBool("build")
	visibility, _ := cmd.Flags().GetString("visibility")
	skipBuild = !build
	skipPush = false
	skipRegister = false
	pushPlatform = "linux/amd64"
	noAuth = false
	serverURL = ""
	registryURL = ""
	pushPublic = visibility == "public"
	pushPrivate = visibility == "private"
	return runPush(cmd, args)
}

// blueprintLatestVersion returns the version with the most recent PublishedAt, or nil if there are none.
func blueprintLatestVersion(versions []blueprintVersionSummary) *blueprintVersionSummary {
	var latest *blueprintVersionSummary
	for i := range versions {
		if latest == nil || versions[i].PublishedAt > latest.PublishedAt {
			latest = &versions[i]
		}
	}
	return latest
}

func blueprintBaseURL() string {
	if blueprintServerURLOverride != "" {
		return strings.TrimSuffix(blueprintServerURLOverride, "/")
	}
	return strings.TrimSuffix(auth.DefaultServerURL, "/")
}

// Response types

type blueprintCard struct {
	Body string `json:"body"`
}

type blueprintVersionSummary struct {
	BuildID     string         `json:"build_id"`
	PublishedAt string         `json:"published_at"`
	Readme      string         `json:"readme,omitempty"`
	AgentCard   *blueprintCard `json:"agent_card,omitempty"`
}

type blueprintMetrics struct {
	LifetimeMessages int64 `json:"lifetime_messages"`
	DeployCount      int64 `json:"deploy_count"`
}

type blueprintItem struct {
	Account    string                    `json:"account"`
	Name       string                    `json:"name"`
	Visibility string                    `json:"visibility"`
	ArchivedAt *time.Time                `json:"archived_at,omitempty"`
	Versions   []blueprintVersionSummary `json:"versions"`
	Metrics    *blueprintMetrics         `json:"metrics"`
	DraftCard  *blueprintCard            `json:"draft_card,omitempty"`
}

// blueprintEffectiveBody mirrors the client's getBlueprintReadme priority:
// latest version's agent_card.body > draft_card.body > latest version's readme.
func blueprintEffectiveBody(bp blueprintItem) string {
	latest := blueprintLatestVersion(bp.Versions)
	if latest != nil && latest.AgentCard != nil && latest.AgentCard.Body != "" {
		return latest.AgentCard.Body
	}
	if bp.DraftCard != nil && bp.DraftCard.Body != "" {
		return bp.DraftCard.Body
	}
	if latest != nil && latest.Readme != "" {
		return latest.Readme
	}
	return ""
}

type listBlueprintsResponse struct {
	Agents []blueprintItem `json:"agents"`
	Count  int             `json:"count"`
}

// --- handlers ---

func runBlueprintList(cmd *cobra.Command, _ []string) error {
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	u := apiPath(blueprintBaseURL(), at.Account, "agents")
	var result listBlueprintsResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Count == 0 {
		fmt.Fprintf(w, "%sNo blueprints found in account %s%s\n", colorDim, at.Account, colorReset) //nolint:errcheck,gosec
		return nil
	}

	const dateFmt = "2006-01-02T15:04:05"
	const dateWidth = len(dateFmt)
	const buildWidth = 8

	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	dim.Fprintf(w, "%-*s  %-*s  %-10s  %-7s  %s\n", dateWidth, "Published", buildWidth, "Build", "Visibility", "Deploys", "Name") //nolint:errcheck,gosec
	for _, bp := range result.Agents {
		latest := blueprintLatestVersion(bp.Versions)
		published, buildID := "pending", ""
		if latest != nil {
			if len(latest.PublishedAt) > dateWidth {
				published = latest.PublishedAt[:dateWidth]
			} else {
				published = latest.PublishedAt
			}
			buildID = latest.BuildID
		}
		deploys := ""
		if bp.Metrics != nil {
			deploys = fmt.Sprintf("%d", bp.Metrics.DeployCount)
		}
		dim.Fprintf(w, "%-*s  %-*s  %-10s  %-7s  ", dateWidth, published, buildWidth, buildID, bp.Visibility, deploys) //nolint:errcheck,gosec
		cyan.Fprintf(w, "%s\n", bp.Name)                                                                               //nolint:errcheck,gosec
	}
	return nil
}

func printBlueprintNextSteps(w io.Writer) {
	boldPrimary := lipgloss.NewStyle().Bold(true).Foreground(theme.Primary)
	dim := lipgloss.NewStyle().Faint(true)

	var lines []string
	lines = append(lines, boldPrimary.Render("Waiting for first push..."))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s   %s", boldPrimary.Render(binaryName+" blueprint push"), dim.Render("push your agent image")))
	lines = append(lines, "")
	lines = append(lines, dim.Render("Tip: use ")+boldPrimary.Render("--build")+dim.Render(" to build the image first."))

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Fprintln(w)      //nolint:errcheck,gosec
	fmt.Fprintln(w, box) //nolint:errcheck,gosec
	fmt.Fprintln(w)      //nolint:errcheck,gosec
}

func runBlueprintCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validateBlueprintName(name); err != nil {
		return err
	}
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	visibility, _ := cmd.Flags().GetString("visibility")
	if err := validateVisibility(visibility); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Creating blueprint %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec

	u := apiPath(blueprintBaseURL(), at.Account, "agents")
	var created struct {
		Account string `json:"account"`
		Name    string `json:"name"`
	}
	status, err := apiCall(cmd.Context(), http.MethodPost, u, map[string]string{
		"name":       name,
		"visibility": visibility,
	}, at.Token, verbose, &created)
	if status == http.StatusConflict {
		return fmt.Errorf("blueprint %q already exists in account %q", name, at.Account)
	}
	if err != nil {
		return err
	}

	printBlueprintNextSteps(w)
	return nil
}

func runBlueprintGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name)
	var bp blueprintItem
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &bp)
	if status == http.StatusNotFound {
		return fmt.Errorf("blueprint %q not found in account %q", name, at.Account)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(bp)
	}

	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	accent := lipgloss.NewStyle().Foreground(theme.Primary)

	fmt.Fprintln(w, bold.Render(bp.Name)+"  "+dim.Render(bp.Account))   //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Visibility:  %s\n", accent.Render(bp.Visibility)) //nolint:errcheck,gosec
	if bp.Metrics != nil {
		fmt.Fprintf(w, "  Deploys:     %d\n", bp.Metrics.DeployCount) //nolint:errcheck,gosec
	}
	if bp.ArchivedAt != nil {
		fmt.Fprintf(w, "  Archived:    %s\n", bp.ArchivedAt.Format(time.RFC3339)) //nolint:errcheck,gosec
	}
	if len(bp.Versions) == 0 {
		printBlueprintNextSteps(w)
	} else {
		fmt.Fprintln(w)                         //nolint:errcheck,gosec
		fmt.Fprintln(w, dim.Render("Versions")) //nolint:errcheck,gosec
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, v := range bp.Versions {
			fmt.Fprintf(tw, "  %s\t%s\n", v.BuildID, v.PublishedAt) //nolint:errcheck,gosec
		}
		tw.Flush() //nolint:errcheck,gosec
	}

	if showCard, _ := cmd.Flags().GetBool("card"); showCard {
		if body := blueprintEffectiveBody(bp); body != "" {
			rendered, err := glamour.Render(body, "auto")
			if err != nil {
				rendered = body
			}
			fmt.Fprintln(w)         //nolint:errcheck,gosec
			fmt.Fprint(w, rendered) //nolint:errcheck,gosec
		}
	}

	return nil
}

func runBlueprintArchive(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Archiving blueprint %s%s%s\n", colorCyan, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "archive")
	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("blueprint %q not found in account %q", name, at.Account)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s archived\n", colorGreen, colorReset, colorBold, name, colorReset) //nolint:errcheck,gosec
	return nil
}

func runBlueprintUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]

	visibility, _ := cmd.Flags().GetString("visibility")
	if visibility == "" {
		return fmt.Errorf("nothing to update — specify --visibility public or --visibility private")
	}
	if err := validateVisibility(visibility); err != nil {
		return err
	}

	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s→%s Setting %s%s%s to %s\n", colorCyan, colorReset, colorBold, name, colorReset, visibility) //nolint:errcheck,gosec

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "visibility")
	status, err := apiCall(cmd.Context(), http.MethodPut, u, map[string]string{
		"visibility": visibility,
	}, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return fmt.Errorf("blueprint %q not found in account %q", name, at.Account)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s%s%s is now %s\n", colorGreen, colorReset, colorBold, name, colorReset, visibility) //nolint:errcheck,gosec
	return nil
}
