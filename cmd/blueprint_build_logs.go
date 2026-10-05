package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// buildLogsPollInterval matches the web UI's poll of a running build's logs.
var buildLogsPollInterval = 3 * time.Second

const minLogOverlapLines = 5

var logSectionHeader = regexp.MustCompile(`^=== (.+) ===$`)

var blueprintBuildsLogsCmd = &cobra.Command{
	Use:   "logs <name> [build-id]",
	Short: "Fetch the logs of a server-side build",
	Long: "Fetch the logs of a build that ran on the server, from a GitHub push or hosted source. " +
		"Without a build ID, uses the newest one. Builds pushed with the CLI run on your machine, so the server has no logs for them.",
	Args: blueprintBuildArgs,
	RunE: runBlueprintBuildsLogs,
}

var blueprintBuildsRebuildCmd = &cobra.Command{
	Use:   "rebuild <name>",
	Short: "Build the head of a blueprint's connected branch on the server",
	Long: "Start a server-side build of the latest commit on the blueprint's connected GitHub branch or hosted source. " +
		"A rebuild cancels any build that is still running.",
	Args: exactValidAgentName,
	RunE: runBlueprintBuildsRebuild,
}

type buildLogComponent struct {
	Name string `json:"name"`
	Logs string `json:"logs"`
}

type buildLogsResponse struct {
	Components []buildLogComponent `json:"components"`
	Phase      string              `json:"phase"`
	Logs       string              `json:"logs"`
}

// A build from before per-component logs reports only the flat field.
func (r buildLogsResponse) components() []buildLogComponent {
	if len(r.Components) > 0 {
		return r.Components
	}
	if r.Logs != "" {
		return []buildLogComponent{{Name: "agent", Logs: r.Logs}}
	}
	return nil
}

type rebuildResponse struct {
	BuildID       string `json:"build_id"`
	CommitSHA     string `json:"commit_sha"`
	CommitMessage string `json:"commit_message"`
	PushedBy      string `json:"pushed_by"`
}

func isRunningBuild(status string) bool {
	return status == "pending" || status == "building"
}

func runBlueprintBuildsLogs(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	var buildID string
	if len(args) == 2 {
		buildID = args[1]
	} else {
		builds, err := fetchBlueprintBuilds(cmd, at, name, 0, verbose)
		if err != nil {
			return err
		}
		if buildID, err = newestServerBuild(builds, name); err != nil {
			return err
		}
	}

	fetch := buildLogsFetcher(cmd.Context(), at, name, buildID, verbose)
	w := cmd.OutOrStdout()
	if tail, _ := cmd.Flags().GetBool("tail"); tail {
		return tailBuildLogs(cmd.Context(), w, buildID, fetch)
	}

	resp, err := fetch()
	if err != nil {
		return err
	}
	components := resp.components()
	if len(components) == 0 {
		fmt.Fprintln(w, colorDim+msgNoBuildLogsYet(buildID, resp.Phase)+colorReset) //nolint:errcheck,gosec
		return nil
	}
	bold := color.New(color.Bold)
	fmt.Fprintf(w, "%s  %s\n", bold.Sprint("Build "+buildID), buildStatusColor(resp.Phase).Sprint(resp.Phase)) //nolint:errcheck,gosec
	for _, c := range components {
		fmt.Fprintln(w)                                  //nolint:errcheck,gosec
		fmt.Fprintln(w, bold.Sprint(c.Name))             //nolint:errcheck,gosec
		fmt.Fprintln(w, strings.TrimRight(c.Logs, "\n")) //nolint:errcheck,gosec
	}
	return nil
}

func runBlueprintBuildsRebuild(cmd *cobra.Command, args []string) error {
	name := args[0]
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	if yes, _ := cmd.Flags().GetBool("yes"); !yes {
		builds, status, err := listBlueprintBuilds(cmd.Context(), at, name, 0, verbose)
		// Without edit access the history is hidden, and the server still authorizes the rebuild.
		if err != nil && status != http.StatusNotFound {
			return err
		}
		for _, b := range builds {
			if isRunningBuild(b.Status) {
				return errRebuildCancelsRunningBuild(b.BuildID, b.Status)
			}
		}
	}

	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "github", "rebuild")
	var started rebuildResponse
	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, &started)
	switch status {
	case http.StatusNotFound:
		return errRebuildUnavailable(name, at.Account)
	case http.StatusUnprocessableEntity:
		return errRebuildGitHubNotConnected()
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s✓%s %s\n", colorGreen, colorReset, msgRebuildStarted(started.BuildID, started.CommitSHA, started.CommitMessage)) //nolint:errcheck,gosec
	if tail, _ := cmd.Flags().GetBool("tail"); !tail {
		fmt.Fprintln(w, colorDim+msgTailBuildLogs(name, started.BuildID)+colorReset) //nolint:errcheck,gosec
		return nil
	}
	fmt.Fprintln(w) //nolint:errcheck,gosec
	return tailBuildLogs(cmd.Context(), w, started.BuildID, buildLogsFetcher(cmd.Context(), at, name, started.BuildID, verbose))
}

// A build can outlast a 5-minute access token, so each poll resolves the token again.
func buildLogsFetcher(ctx context.Context, at AccountToken, name, buildID string, verbose bool) func() (buildLogsResponse, error) {
	return func() (buildLogsResponse, error) {
		token, err := getAccountToken(ctx, at.Account)
		if err != nil {
			return buildLogsResponse{}, err
		}
		return fetchBuildLogs(ctx, AccountToken{Account: at.Account, Token: token}, name, buildID, verbose)
	}
}

func fetchBuildLogs(ctx context.Context, at AccountToken, name, buildID string, verbose bool) (buildLogsResponse, error) {
	u := apiPath(blueprintBaseURL(), at.Account, "agents", name, "github", "builds", url.PathEscape(buildID), "logs")
	var resp buildLogsResponse
	status, err := apiCall(ctx, http.MethodGet, u, nil, at.Token, verbose, &resp)
	if status == http.StatusNotFound {
		return resp, errBuildLogsNotFound(name, buildID)
	}
	return resp, err
}

func newestServerBuild(builds []blueprintBuild, name string) (string, error) {
	for _, b := range builds {
		if b.Source != "cli" {
			return b.BuildID, nil
		}
	}
	return "", errBlueprintNoServerBuilds(name)
}

// Each poll returns the whole log again, so this diffs it per section.
func tailBuildLogs(ctx context.Context, w io.Writer, buildID string, fetch func() (buildLogsResponse, error)) error {
	printed := map[string][]string{}
	var lastComponent, lastSection string
	for {
		resp, err := fetch()
		if err != nil {
			return err
		}
		for _, c := range resp.components() {
			for _, s := range logSections(c.Logs) {
				key := c.Name + "\x00" + s.title
				fresh := unseenLines(printed[key], s.lines)
				printed[key] = s.lines
				if len(fresh) == 0 {
					continue
				}
				if c.Name != lastComponent {
					fmt.Fprintln(w, color.New(color.Bold).Sprint(c.Name)) //nolint:errcheck,gosec
					lastComponent, lastSection = c.Name, ""
				}
				if key != lastSection && s.title != "" {
					fmt.Fprintf(w, "=== %s ===\n", s.title) //nolint:errcheck,gosec
				}
				lastSection = key
				fmt.Fprintln(w, strings.Join(fresh, "\n")) //nolint:errcheck,gosec
			}
		}
		if !isRunningBuild(resp.Phase) {
			return buildOutcome(w, buildID, resp.Phase)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(buildLogsPollInterval):
		}
	}
}

func buildOutcome(w io.Writer, buildID, phase string) error {
	switch phase {
	case "failed":
		return errBuildFailed(buildID)
	case "cancelled":
		return errBuildCancelled(buildID)
	}
	fmt.Fprintln(w, buildStatusColor(phase).Sprint(msgBuildFinished(buildID, phase))) //nolint:errcheck,gosec
	return nil
}

type logSection struct {
	title string
	lines []string
}

// A container that has not started reports one "(...)" placeholder line, which
// counts as no output so the tail does not print it.
func logSections(text string) []logSection {
	var sections []logSection
	current := logSection{}
	flush := func() {
		lines := current.lines
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) == 1 && strings.HasPrefix(lines[0], "(") && strings.HasSuffix(lines[0], ")") {
			lines = nil
		}
		if current.title != "" || len(lines) > 0 {
			sections = append(sections, logSection{title: current.title, lines: lines})
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if m := logSectionHeader.FindStringSubmatch(line); m != nil {
			flush()
			current = logSection{title: m[1]}
			continue
		}
		current.lines = append(current.lines, line)
	}
	flush()
	return sections
}

// The server sends each container's last 500 lines, so once that window slides
// cur starts partway into prev. A shorter overlap than minLogOverlapLines is a rewrite.
func unseenLines(prev, cur []string) []string {
	minOverlap := min(len(prev), minLogOverlapLines)
	for start := 0; len(prev)-start >= minOverlap && len(prev) > 0; start++ {
		overlap := prev[start:]
		if len(cur) >= len(overlap) && slices.Equal(cur[:len(overlap)], overlap) {
			return cur[len(overlap):]
		}
	}
	return cur
}

func shortSHA(sha string) string {
	return sha[:min(len(sha), 7)]
}
