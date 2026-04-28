package cmd

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// blueprintServerURLOverride is set in tests to redirect API calls to a test server.
var blueprintServerURLOverride string

func exactValidAgentName(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	if err := spec.ValidateName(args[0]); err != nil {
		return fmt.Errorf("agent name %q: %w", args[0], err)
	}
	return nil
}

// Visibility is the access-control level of an agent blueprint.
type Visibility string

const (
	VisibilityUnset   Visibility = "" // preserve existing; only meaningful in push context
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// ParseVisibility reads the --visibility flag from cmd and converts it to a Visibility.
// Empty string maps to VisibilityUnset (preserve existing); non-empty must be "public" or "private".
func ParseVisibility(cmd *cobra.Command) (Visibility, error) {
	s, _ := cmd.Flags().GetString("visibility")
	switch s {
	case "":
		if cmd.Flags().Changed("visibility") {
			return "", fmt.Errorf("--visibility must be 'public' or 'private', got %q", s)
		}
		return VisibilityUnset, nil
	case "public":
		return VisibilityPublic, nil
	case "private":
		return VisibilityPrivate, nil
	default:
		return "", fmt.Errorf("--visibility must be 'public' or 'private', got %q", s)
	}
}

var blueprintCmd = &cobra.Command{
	Use:     "blueprint",
	Aliases: []string{"bp"},
	Short:   "Manage agent blueprints",
	Long:    "Manage agent blueprints as registered, versioned agent definitions on the platform.",
	Args:    cobra.NoArgs,
	RunE:    func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var blueprintListCmd = &cobra.Command{
	Use:   "list",
	Short: "List blueprints in the active account",
	Args:  cobra.NoArgs,
	RunE:  runBlueprintList,
}

var blueprintCreateCmd = &cobra.Command{
	Use:    "create <name>",
	Short:  "Register a new blueprint on the server",
	Args:   exactValidAgentName,
	RunE:   runBlueprintCreate,
	Hidden: true,
}

var blueprintGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get blueprint metadata and config",
	Args:  exactValidAgentName,
	RunE:  runBlueprintGet,
}

var blueprintPushCmd = &cobra.Command{
	Use:   "push <name>",
	Short: "Push blueprint image to registry",
	Long:  "Push blueprint image to registry. If the blueprint does not yet exist it will be created automatically.",
	Args:  exactValidAgentName,
	RunE:  runBlueprintPush,
}

var blueprintArchiveCmd = &cobra.Command{
	Use:   "archive <name>",
	Short: "Archive a blueprint",
	Args:  exactValidAgentName,
	RunE:  runBlueprintArchive,
}

var blueprintBuildCmd = &cobra.Command{
	Use:   "build <name>",
	Short: "Build blueprint image",
	Long:  "Build the agent blueprint image. Use 'blueprint push' to push it to the registry.",
	Args:  exactValidAgentName,
	RunE:  runBlueprintBuild,
}

var blueprintSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Update blueprint settings",
	Args:  exactValidAgentName,
	RunE:  runBlueprintSet,
}

// registerPushFlags adds push flags to any command that invokes runPush.
func registerPushFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	cmd.Flags().Bool("build", false, "Build image before pushing")
	cmd.Flags().StringP("visibility", "V", "", "Set visibility: public or private")
	cmd.Flags().Bool("allow-account-override", false, "Allow push when the account prefix in the spec differs from the current account")
}

func init() {
	rootCmd.AddCommand(blueprintCmd)
	blueprintCmd.AddCommand(blueprintListCmd)
	blueprintCmd.AddCommand(blueprintCreateCmd)
	blueprintCmd.AddCommand(blueprintGetCmd)
	blueprintCmd.AddCommand(blueprintBuildCmd)
	blueprintCmd.AddCommand(blueprintPushCmd)
	blueprintCmd.AddCommand(blueprintArchiveCmd)
	blueprintCmd.AddCommand(blueprintSetCmd)
	blueprintListCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintGetCmd.Flags().Bool("card", false, "Show agent description")
	blueprintGetCmd.Flags().Bool("template", false, "Show deployment variables and secrets")

	blueprintCreateCmd.Flags().StringP("visibility", "V", string(VisibilityPrivate), "Set visibility: public or private")
	blueprintSetCmd.Flags().StringP("visibility", "V", "", "Set visibility: public or private")
	registerPushFlags(blueprintPushCmd)
	blueprintBuildCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")

	// Top-level aliases
	topLevelBuildCmd := &cobra.Command{
		Use:   "build <name>",
		Short: blueprintBuildCmd.Short,
		Long:  blueprintBuildCmd.Long,
		Args:  exactValidAgentName,
		RunE:  runBlueprintBuild,
	}
	topLevelBuildCmd.Flags().StringP("file", "f", "", "Path to spec file (default: astropods.yml)")
	rootCmd.AddCommand(topLevelBuildCmd)

	topLevelPushCmd := &cobra.Command{
		Use:   "push <name>",
		Short: blueprintPushCmd.Short,
		Long:  blueprintPushCmd.Long,
		Args:  exactValidAgentName,
		RunE:  runBlueprintPush,
	}
	rootCmd.AddCommand(topLevelPushCmd)
	registerPushFlags(topLevelPushCmd)
}

func runBlueprintBuild(cmd *cobra.Command, args []string) error {
	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
	if err != nil {
		return err
	}
	if _, err := validateSpecFile(specPath); err != nil {
		return err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	platform, _ := resolveBuildPlatform(auth.DefaultServerURL)
	return runBuild(cmd.Context(), specPath, args[0], generateBuildID(), []string{platform}, false, verbose, false)
}

func runBlueprintPush(cmd *cobra.Command, args []string) error {
	vis, err := ParseVisibility(cmd)
	if err != nil {
		return err
	}
	specPath, err := resolveSpecPathFromCwd(flagString(cmd, "file"))
	if err != nil {
		return err
	}
	// Validate spec before auth so bad specs produce a useful error immediately.
	if _, err := validateSpecFile(specPath); err != nil {
		return err
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	build, _ := cmd.Flags().GetBool("build")
	allowAccountOverride, _ := cmd.Flags().GetBool("allow-account-override")
	platform, skipPush := resolveBuildPlatform(pushBaseURL())
	return runPush(cmd.Context(), at, pushConfig{
		specPath:             specPath,
		agentName:            args[0],
		skipBuild:            !build,
		skipPush:             skipPush,
		platform:             platform,
		visibility:           vis,
		allowAccountOverride: allowAccountOverride,
		verbose:              verbose,
	})
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

type templateVariable struct {
	Targets     []string `json:"targets"`
	Secret      bool     `json:"secret"`
	Optional    bool     `json:"optional"`
	Default     string   `json:"default,omitempty"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
}

type deploymentTemplateResponse struct {
	Variables map[string]templateVariable `json:"variables"`
}

// --- handlers ---

func runBlueprintList(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	u := apiPath(blueprintBaseURL(), at.Account, "agents")
	var result listBlueprintsResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	w := cmd.OutOrStdout()

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	if result.Count == 0 {
		fmt.Fprintf(w, "%sNo blueprints found in account %s%s\n", colorDim, at.Account, colorReset) //nolint:errcheck,gosec
		return nil
	}

	cyan := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)

	dim.Fprintf(w, "%-*s  %-*s  %-10s  %-7s  %s\n", tableTimeWidth, "Published", tableBuildWidth, "Build", "Visibility", "Deploys", "Name") //nolint:errcheck,gosec
	for _, bp := range result.Agents {
		latest := blueprintLatestVersion(bp.Versions)
		published, buildID := "pending", ""
		if latest != nil {
			published = truncate(latest.PublishedAt, tableTimeWidth)
			buildID = latest.BuildID
		}
		deploys := ""
		if bp.Metrics != nil {
			deploys = fmt.Sprintf("%d", bp.Metrics.DeployCount)
		}
		dim.Fprintf(w, "%-*s  %-*s  %-10s  %-7s  ", tableTimeWidth, published, tableBuildWidth, buildID, bp.Visibility, deploys) //nolint:errcheck,gosec
		cyan.Fprintf(w, "%s\n", bp.Name)                                                                                         //nolint:errcheck,gosec
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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	vis, err := ParseVisibility(cmd)
	if err != nil {
		return err
	}
	visibility := string(vis)

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
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

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
		return writeJSON(w, bp)
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

	if showTemplate, _ := cmd.Flags().GetBool("template"); showTemplate {
		tu := apiPath(blueprintBaseURL(), at.Account, "agents", name, "deployment-template")
		var tmpl deploymentTemplateResponse
		if _, err := apiCall(cmd.Context(), http.MethodPost, tu, map[string]any{}, at.Token, verbose, &tmpl); err != nil {
			return err
		}
		printDeploymentTemplate(w, tmpl, dim)
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

const varDescMaxLen = 60

func printDeploymentTemplate(w io.Writer, tmpl deploymentTemplateResponse, dim lipgloss.Style) {
	fmt.Fprintln(w)                                                            //nolint:errcheck,gosec
	fmt.Fprintln(w, dim.Render("Variables and secrets needed for deployment")) //nolint:errcheck,gosec

	// Sort variables: required first, then optional; alphabetical within each group.
	var required, optional []string
	for k, v := range tmpl.Variables {
		if v.Optional {
			optional = append(optional, k)
		} else {
			required = append(required, k)
		}
	}
	sort.Strings(required)
	sort.Strings(optional)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	writeVarRows(tw, required, tmpl.Variables, false)
	writeVarRows(tw, optional, tmpl.Variables, true)
	tw.Flush() //nolint:errcheck,gosec
}

func writeVarRows(tw *tabwriter.Writer, keys []string, vars map[string]templateVariable, isOptional bool) {
	for _, k := range keys {
		v := vars[k]
		var notes []string
		if v.Secret {
			notes = append(notes, "secret")
		}
		if isOptional {
			if v.Default != "" {
				notes = append(notes, "default: "+v.Default)
			} else {
				notes = append(notes, "optional")
			}
		}
		suffix := ""
		if len(notes) > 0 {
			suffix = "(" + strings.Join(notes, ", ") + ")"
		}
		desc := v.Description
		if v.Label != "" && v.Label != k {
			desc = v.Label
			if v.Description != "" {
				desc += " — " + v.Description
			}
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", k, truncate(desc, varDescMaxLen), suffix) //nolint:errcheck,gosec
	}
}

func runBlueprintArchive(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

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

func runBlueprintSet(cmd *cobra.Command, args []string) error {
	name := args[0]

	vis, err := ParseVisibility(cmd)
	if err != nil {
		return err
	}
	if vis == VisibilityUnset {
		return fmt.Errorf("nothing to update — specify --visibility public or --visibility private")
	}
	visibility := string(vis)

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

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
