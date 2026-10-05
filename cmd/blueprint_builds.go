package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	spec "github.com/astropods/astro-spec"
)

const findingTitleMaxLen = 60

var blueprintBuildsCmd = &cobra.Command{
	Use:   "builds <name>",
	Short: "List a blueprint's builds and their image vulnerabilities",
	Args:  exactValidAgentName,
	RunE:  runBlueprintBuilds,
}

var blueprintVulnerabilitiesCmd = &cobra.Command{
	Use:     "vulnerabilities <name> [build-id]",
	Aliases: []string{"vulns"},
	Short:   "Show the image vulnerabilities found in a build",
	Long:    "Show the image vulnerabilities found in a build. Without a build ID, shows the latest published build.",
	Args:    blueprintVulnerabilitiesArgs,
	RunE:    runBlueprintVulnerabilities,
}

func init() {
	blueprintCmd.AddCommand(blueprintBuildsCmd)
	blueprintCmd.AddCommand(blueprintVulnerabilitiesCmd)
	blueprintBuildsCmd.Flags().Int("limit", 0, "Maximum builds to list (server default 50, max 200)")
	blueprintBuildsCmd.Flags().Bool("json", false, "Print raw JSON output")
	blueprintVulnerabilitiesCmd.Flags().Bool("json", false, "Print raw JSON output")
}

func blueprintVulnerabilitiesArgs(_ *cobra.Command, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errBlueprintVulnerabilitiesArgs(len(args))
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

func runBlueprintBuilds(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "builds")
	if limit, _ := cmd.Flags().GetInt("limit"); limit > 0 {
		u += "?limit=" + strconv.Itoa(limit)
	}
	var result blueprintBuildsResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result)
	// The server answers 404 for a blueprint the caller can read but not edit.
	if status == http.StatusNotFound {
		return errBlueprintBuildsNotFound(name, at.Account)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}
	if len(result.Builds) == 0 {
		fmt.Fprintln(w, colorDim+msgNoBlueprintBuilds(name)+colorReset) //nolint:errcheck,gosec
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "BUILD\tSTARTED\tSOURCE\tSTATUS\tVULNERABILITIES") //nolint:errcheck,gosec
	for _, b := range result.Builds {
		buildStatus := b.Status
		if b.IsLatest {
			buildStatus += " (latest)"
		}
		summary := "-"
		if b.Vulnerabilities != nil {
			summary = colorVulnerabilitySummary(b.Vulnerabilities.Status, b.Vulnerabilities.vulnerabilityCounts)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", b.BuildID, b.StartedAt, b.Source, buildStatus, summary) //nolint:errcheck,gosec
	}
	return tw.Flush()
}

func runBlueprintVulnerabilities(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	var buildID string
	if len(args) == 2 {
		buildID = args[1]
	} else if buildID, err = latestBlueprintBuild(cmd, at, name, verbose); err != nil {
		return err
	}

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "builds", url.PathEscape(buildID), "vulnerabilities")
	var result buildVulnerabilitiesResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result)
	if status == http.StatusNotFound {
		return errBlueprintNotFound(name, at.Account)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}
	if result.Summary == nil || len(result.Components) == 0 {
		fmt.Fprintln(w, colorDim+msgNoBuildScans(name, buildID)+colorReset) //nolint:errcheck,gosec
		return nil
	}

	bold := color.New(color.Bold)
	dim := color.New(color.Faint)
	summary := colorVulnerabilitySummary(result.Summary.Status, result.Summary.vulnerabilityCounts)
	if result.Summary.Fixable > 0 {
		summary += dim.Sprintf(" (%d fixable)", result.Summary.Fixable)
	}
	fmt.Fprintf(w, "%s %s  %s\n", bold.Sprint(name), dim.Sprint("build "+buildID), summary) //nolint:errcheck,gosec

	for _, c := range result.Components {
		fmt.Fprintln(w) //nolint:errcheck,gosec
		printComponentVulnerabilities(w, c)
	}
	return nil
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
