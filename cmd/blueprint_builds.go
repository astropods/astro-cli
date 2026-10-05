package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	spec "github.com/astropods/astro-spec"
)

const (
	findingTitleMaxLen = 60
	commitTitleMaxLen  = 60
	// The server caps one page of build history at 200.
	maxBuildListLimit = 200
)

var blueprintBuildsCmd = &cobra.Command{
	Use:   "builds",
	Short: "List and inspect a blueprint's builds",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var blueprintBuildsListCmd = &cobra.Command{
	Use:   "list <name>",
	Short: "List a blueprint's builds, newest first",
	Args:  exactValidAgentName,
	RunE:  runBlueprintBuildsList,
}

var blueprintBuildsGetCmd = &cobra.Command{
	Use:   "get <name> [build-id]",
	Short: "Show a build's details and image vulnerabilities",
	Long:  "Show a build's details and the vulnerabilities found in its images. Without a build ID, shows the latest published build.",
	Args:  blueprintBuildArgs,
	RunE:  runBlueprintBuildsGet,
}

func init() {
	blueprintCmd.AddCommand(blueprintBuildsCmd)
	blueprintBuildsCmd.AddCommand(blueprintBuildsListCmd)
	blueprintBuildsCmd.AddCommand(blueprintBuildsGetCmd)
	blueprintBuildsCmd.AddCommand(blueprintBuildsLogsCmd)
	blueprintBuildsCmd.AddCommand(blueprintBuildsRebuildCmd)
	blueprintBuildsListCmd.Flags().Int("limit", 0, "Maximum builds to list (server default 50, max 200)")
	blueprintBuildsListCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintBuildsGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintBuildsLogsCmd.Flags().BoolP("tail", "t", false, "Stream logs until the build finishes")
	blueprintBuildsRebuildCmd.Flags().BoolP("tail", "t", false, "Stream the new build's logs until it finishes")
	blueprintBuildsRebuildCmd.Flags().BoolP("yes", "y", false, "Rebuild even when a build is still running")
}

func blueprintBuildArgs(_ *cobra.Command, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errBlueprintBuildArgs(len(args))
	}
	if err := spec.ValidateName(args[0]); err != nil {
		return fmt.Errorf("blueprint name %q: %w", args[0], err)
	}
	return nil
}

type vulnerabilityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
	Fixable  int `json:"fixable"`
}

type buildVulnerabilitySummary struct {
	Status string `json:"status"`
	vulnerabilityCounts
}

type blueprintBuild struct {
	BuildID         string                     `json:"build_id"`
	Source          string                     `json:"source"`
	Status          string                     `json:"status"`
	StartedAt       string                     `json:"started_at"`
	CompletedAt     string                     `json:"completed_at,omitempty"`
	Step            string                     `json:"step,omitempty"`
	Error           string                     `json:"error,omitempty"`
	IsLatest        bool                       `json:"is_latest"`
	CommitSHA       string                     `json:"commit_sha,omitempty"`
	CommitMessage   string                     `json:"commit_message,omitempty"`
	Branch          string                     `json:"branch,omitempty"`
	RepoFullName    string                     `json:"repo_full_name,omitempty"`
	PushedBy        string                     `json:"pushed_by,omitempty"`
	PushedByHandle  string                     `json:"pushed_by_handle,omitempty"`
	Vulnerabilities *buildVulnerabilitySummary `json:"vulnerabilities,omitempty"`
}

type blueprintBuildsResponse struct {
	Builds []blueprintBuild `json:"builds"`
}

type vulnerabilityFinding struct {
	ID               string `json:"id"`
	Severity         string `json:"severity"`
	Package          string `json:"package"`
	PackageType      string `json:"package_type,omitempty"`
	InstalledVersion string `json:"installed_version,omitempty"`
	FixedVersion     string `json:"fixed_version,omitempty"`
	Title            string `json:"title,omitempty"`
	URL              string `json:"url,omitempty"`
}

type componentVulnerabilities struct {
	Component   string                 `json:"component"`
	Status      string                 `json:"status"`
	ImageDigest string                 `json:"image_digest,omitempty"`
	ScannedAt   string                 `json:"scanned_at,omitempty"`
	DBUpdatedAt string                 `json:"db_updated_at,omitempty"`
	Counts      vulnerabilityCounts    `json:"counts"`
	Findings    []vulnerabilityFinding `json:"findings"`
}

type buildVulnerabilitiesResponse struct {
	BuildID    string                     `json:"build_id"`
	Summary    *buildVulnerabilitySummary `json:"summary"`
	Components []componentVulnerabilities `json:"components"`
}

func listBlueprintBuilds(ctx context.Context, at AccountToken, name string, limit int, verbose bool) ([]blueprintBuild, int, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "builds")
	if limit > 0 {
		u += "?limit=" + strconv.Itoa(limit)
	}
	var result blueprintBuildsResponse
	status, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &result)
	return result.Builds, status, err
}

func fetchBlueprintBuilds(cmd *cobra.Command, at AccountToken, name string, limit int, verbose bool) ([]blueprintBuild, error) {
	builds, status, err := listBlueprintBuilds(cmd.Context(), at, name, limit, verbose)
	// The server answers 404 for a blueprint the caller can read but not edit.
	if status == http.StatusNotFound {
		return nil, errBlueprintBuildsNotFound(name, at.Account)
	}
	return builds, err
}

func runBlueprintBuildsList(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
	builds, err := fetchBlueprintBuilds(cmd, at, name, limit, verbose)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, blueprintBuildsResponse{Builds: builds})
	}
	if len(builds) == 0 {
		fmt.Fprintln(w, colorDim+msgNoBlueprintBuilds(name)+colorReset) //nolint:errcheck,gosec
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BUILD\tSTARTED\tSOURCE\tSTATUS\tVULNERABILITIES") //nolint:errcheck,gosec
	for _, b := range builds {
		summary := "-"
		if b.Vulnerabilities != nil {
			summary = colorVulnerabilitySummary(b.Vulnerabilities.Status, b.Vulnerabilities.vulnerabilityCounts)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", b.BuildID, b.StartedAt, b.Source, buildStatusText(b), summary) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

func runBlueprintBuildsGet(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	builds, err := fetchBlueprintBuilds(cmd, at, name, maxBuildListLimit, verbose)
	if err != nil {
		return err
	}
	var buildID string
	if len(args) == 2 {
		buildID = args[1]
	}
	build, err := selectBuild(builds, name, buildID)
	if err != nil {
		return err
	}

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "builds", url.PathEscape(build.BuildID), "vulnerabilities")
	var vulns buildVulnerabilitiesResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &vulns); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, buildDetailResponse{Build: build, Vulnerabilities: vulns})
	}
	printBuildDetails(w, build)
	fmt.Fprintln(w) //nolint:errcheck,gosec
	printBuildVulnerabilities(w, vulns)
	return nil
}

type buildDetailResponse struct {
	Build           blueprintBuild               `json:"build"`
	Vulnerabilities buildVulnerabilitiesResponse `json:"vulnerabilities"`
}

func selectBuild(builds []blueprintBuild, name, buildID string) (blueprintBuild, error) {
	for _, b := range builds {
		if buildID == "" && b.IsLatest || buildID != "" && b.BuildID == buildID {
			return b, nil
		}
	}
	if buildID == "" {
		return blueprintBuild{}, errBlueprintNoPublishedBuild(name)
	}
	return blueprintBuild{}, errBuildNotFound(name, buildID, maxBuildListLimit)
}

func buildStatusText(b blueprintBuild) string {
	if b.IsLatest {
		return b.Status + " (latest)"
	}
	return b.Status
}

func buildStatusColor(status string) *color.Color {
	switch status {
	case "registered":
		return color.New(color.FgGreen)
	case "failed":
		return color.New(color.FgRed)
	case "pending", "building":
		return color.New(color.FgYellow)
	default:
		return color.New(color.Faint)
	}
}

func printBuildDetails(w io.Writer, b blueprintBuild) {
	bold := color.New(color.Bold)
	fmt.Fprintf(w, "%s  %s\n", bold.Sprint("Build "+b.BuildID), buildStatusColor(b.Status).Sprint(buildStatusText(b))) //nolint:errcheck,gosec

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	row := func(label, value string) {
		if value != "" {
			fmt.Fprintf(tw, "  %s:\t%s\n", label, value) //nolint:errcheck,gosec
		}
	}
	row("Source", b.Source)
	row("Started", b.StartedAt)
	row("Completed", buildCompletedText(b))
	row("Step", b.Step)
	if b.Error != "" {
		row("Error", color.New(color.FgRed).Sprint(b.Error))
	}
	if b.CommitSHA != "" {
		row("Commit", strings.TrimSpace(shortSHA(b.CommitSHA)+" "+commitTitle(b.CommitMessage)))
	}
	row("Branch", b.Branch)
	row("Repository", b.RepoFullName)
	row("Pushed by", pushedByText(b))
	tw.Flush() //nolint:errcheck,gosec
}

func buildCompletedText(b blueprintBuild) string {
	if b.CompletedAt == "" {
		return ""
	}
	started, err1 := time.Parse(time.RFC3339, b.StartedAt)
	completed, err2 := time.Parse(time.RFC3339, b.CompletedAt)
	if err1 != nil || err2 != nil {
		return b.CompletedAt
	}
	return fmt.Sprintf("%s (%s)", b.CompletedAt, completed.Sub(started).Round(time.Second))
}

func commitTitle(message string) string {
	title, _, _ := strings.Cut(message, "\n")
	return truncate(strings.TrimSpace(title), commitTitleMaxLen)
}

func pushedByText(b blueprintBuild) string {
	if b.PushedByHandle != "" && b.PushedBy != "" {
		return fmt.Sprintf("%s (@%s)", b.PushedBy, b.PushedByHandle)
	}
	if b.PushedBy != "" {
		return b.PushedBy
	}
	if b.PushedByHandle != "" {
		return "@" + b.PushedByHandle
	}
	return ""
}

func printBuildVulnerabilities(w io.Writer, v buildVulnerabilitiesResponse) {
	bold := color.New(color.Bold)
	if v.Summary == nil || len(v.Components) == 0 {
		fmt.Fprintf(w, "%s  %s\n", bold.Sprint("Vulnerabilities"), color.New(color.Faint).Sprint(vulnerabilitySummary("", vulnerabilityCounts{}))) //nolint:errcheck,gosec
		return
	}
	summary := colorVulnerabilitySummary(v.Summary.Status, v.Summary.vulnerabilityCounts)
	if v.Summary.Fixable > 0 {
		summary += color.New(color.Faint).Sprintf(" (%d fixable)", v.Summary.Fixable)
	}
	fmt.Fprintf(w, "%s  %s\n", bold.Sprint("Vulnerabilities"), summary) //nolint:errcheck,gosec
	for _, c := range v.Components {
		fmt.Fprintln(w) //nolint:errcheck,gosec
		printComponentVulnerabilities(w, c)
	}
}

func printComponentVulnerabilities(w io.Writer, c componentVulnerabilities) {
	bold := color.New(color.Bold)
	dim := color.New(color.Faint)

	var meta []string
	if c.ImageDigest != "" {
		meta = append(meta, shortImageDigest(c.ImageDigest))
	}
	if c.ScannedAt != "" {
		meta = append(meta, "scanned "+c.ScannedAt)
	}
	fmt.Fprintf(w, "%s  %s  %s\n", bold.Sprint(c.Component), colorVulnerabilitySummary(c.Status, c.Counts), dim.Sprint(strings.Join(meta, "  "))) //nolint:errcheck,gosec

	if len(c.Findings) == 0 {
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  SEVERITY\tID\tPACKAGE\tINSTALLED\tFIXED\tTITLE") //nolint:errcheck,gosec
	for _, f := range c.Findings {
		fixed := f.FixedVersion
		if fixed == "" {
			fixed = "-"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", f.Severity, f.ID, f.Package, f.InstalledVersion, fixed, truncate(f.Title, findingTitleMaxLen)) //nolint:errcheck,gosec
	}
	tw.Flush() //nolint:errcheck,gosec
}

// A running or failed scan has partial counts, so it shows its state instead.
func vulnerabilitySummary(status string, c vulnerabilityCounts) string {
	switch status {
	case "", "skipped":
		return "not scanned"
	case "pending", "scanning":
		return "scanning"
	case "failed":
		return "scan failed"
	}
	var parts []string
	for _, s := range []struct {
		n     int
		label string
	}{{c.Critical, "critical"}, {c.High, "high"}, {c.Medium, "medium"}, {c.Low, "low"}, {c.Unknown, "unknown"}} {
		if s.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", s.n, s.label))
		}
	}
	if len(parts) == 0 {
		return "none found"
	}
	return strings.Join(parts, ", ")
}

// Use only in a tabwriter's last column: escape codes count toward cell width.
func colorVulnerabilitySummary(status string, c vulnerabilityCounts) string {
	text := vulnerabilitySummary(status, c)
	switch {
	case status == "failed" || (status == "succeeded" && (c.Critical > 0 || c.High > 0)):
		return color.New(color.FgRed).Sprint(text)
	case status == "succeeded" && c.Medium > 0:
		return color.New(color.FgYellow).Sprint(text)
	case status == "succeeded" && c.Low+c.Unknown == 0:
		return color.New(color.FgGreen).Sprint(text)
	}
	return text
}

func shortImageDigest(digest string) string {
	algo, sum, ok := strings.Cut(digest, ":")
	if !ok {
		return truncate(digest, 12)
	}
	if len(sum) > 12 {
		sum = sum[:12]
	}
	return algo + ":" + sum
}
