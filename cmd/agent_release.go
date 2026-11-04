package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type releasePolicy struct {
	DeploymentID      string `json:"deployment_id"`
	Enabled           bool   `json:"enabled"`
	Version           string `json:"version"`
	Mode              string `json:"mode"`
	CandidateID       string `json:"candidate_id,omitempty"`
	AcceptanceVersion string `json:"acceptance_version,omitempty"`
	DesiredBuild      string `json:"desired_build_id,omitempty"`
	SubmittedRevision *int   `json:"submitted_revision,omitempty"`
	AcceptedRunID     string `json:"accepted_run_id,omitempty"`
	Outcome           string `json:"outcome"`
	Message           string `json:"message,omitempty"`
}

type releaseStatus struct {
	Policy   releasePolicy `json:"policy"`
	Evidence *struct {
		RunID  string `json:"run_id,omitempty"`
		Reason string `json:"reason"`
	} `json:"evidence,omitempty"`
	Ready bool `json:"ready"`
}

func newAgentReleaseCommand() *cobra.Command {
	parent := &cobra.Command{Use: "release", Short: "Inspect and recover automatic deployments"}
	parent.AddCommand(newReleaseSetupCommand(), newReleaseCompareCommand())
	status := &cobra.Command{Use: "status", Short: "Show publication, evaluation, and rollout progress", Args: cobra.NoArgs, RunE: runReleaseStatus}
	history := &cobra.Command{Use: "history", Short: "List a candidate's evaluation runs or policy versions", Args: cobra.NoArgs, RunE: runReleaseHistory}
	retry := &cobra.Command{Use: "retry", Short: "Retry a blocked publication after resolving its cause", Args: cobra.NoArgs, RunE: runReleaseRetry}
	for _, command := range []*cobra.Command{status, history, retry} {
		command.Flags().String("id", "", "Deployment ID")
		command.Flags().Bool("json", false, "Print JSON output")
		parent.AddCommand(command)
	}
	history.Flags().Int("limit", 20, "Page size, from 1 to 50")
	history.Flags().String("cursor", "", "Cursor from the preceding page")
	history.Flags().Bool("policies", false, "List acceptance policy versions")
	retry.Flags().String("build", "", "Expected published build ID")
	retry.Flags().String("policy-version", "", "Expected policy version (defaults to the current version)")
	retry.Flags().String("request-id", "", "Stable UUID for resuming a retry request")
	return parent
}

func init() { agentCmd.AddCommand(newAgentReleaseCommand()) }

func releaseDeploymentID(cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString("id")
	if id == "" {
		return "", errReleaseDeploymentRequired()
	}
	return id, nil
}

func releaseURL(id string, parts ...string) string {
	return apiPath(agentBaseURL(), url.PathEscape(id), "deployments", parts...)
}

func runReleaseStatus(cmd *cobra.Command, _ []string) error {
	id, err := releaseDeploymentID(cmd)
	if err != nil {
		return err
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var result releaseStatus
	if _, err = apiCall(cmd.Context(), http.MethodGet, releaseURL(id, "auto-deploy", "status"), nil, at.Token, verbose, &result); err != nil {
		return err
	}
	p := result.Policy
	if p.Outcome == "submitted" && p.SubmittedRevision != nil {
		// The server's flag cannot describe a target that has since moved.
		result.Ready = false
		dep, err := getAgentDeploymentFull(cmd.Context(), id, at, verbose)
		if err != nil {
			return err
		}
		if dep.BuildID == p.DesiredBuild && dep.CurrentRevision == *p.SubmittedRevision {
			runtime, err := pollDeploymentRuntime(cmd.Context(), id, at, verbose)
			if err != nil {
				return err
			}
			result.Ready = runtime.ReadyRevision == *p.SubmittedRevision
		}
	}
	if output, _ := cmd.Flags().GetBool("json"); output {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgReleaseStatus(result))
	return err
}

type releaseHistoryItem struct {
	ID        string `json:"id,omitempty"`
	Version   string `json:"version,omitempty"`
	BuildID   string `json:"build_id,omitempty"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at"`
	Decision  *struct {
		Accepted      bool   `json:"accepted"`
		Reason        string `json:"reason"`
		PolicyVersion string `json:"policy_version"`
	} `json:"decision,omitempty"`
}

type releaseHistory struct {
	Items      []releaseHistoryItem `json:"items"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

func runReleaseHistory(cmd *cobra.Command, _ []string) error {
	id, err := releaseDeploymentID(cmd)
	if err != nil {
		return err
	}
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > 50 {
		return errReleaseHistoryLimit()
	}
	cursor, _ := cmd.Flags().GetString("cursor")
	policies, _ := cmd.Flags().GetBool("policies")
	path := "build-evaluations"
	if policies {
		path = "build-acceptance-policies"
	}
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var raw json.RawMessage
	if _, err = apiCall(cmd.Context(), http.MethodGet, releaseURL(id, path)+"?"+q.Encode(), nil, at.Token, verbose, &raw); err != nil {
		return err
	}
	if output, _ := cmd.Flags().GetBool("json"); output {
		return writeJSON(cmd.OutOrStdout(), raw)
	}
	var page releaseHistory
	if err = json.Unmarshal(raw, &page); err != nil {
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	if _, err = fmt.Fprintln(w, msgReleaseHistoryHeader()); err != nil {
		return err
	}
	for _, item := range page.Items {
		id := item.ID
		if id == "" {
			id = item.Version
		}
		if _, err = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", id, item.BuildID, item.Status, msgReleaseHistoryDecision(item), item.CreatedAt); err != nil {
			return err
		}
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if page.NextCursor != "" {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), msgReleaseHistoryCursor(page.NextCursor))
	}
	return err
}

func runReleaseRetry(cmd *cobra.Command, _ []string) error {
	id, err := releaseDeploymentID(cmd)
	if err != nil {
		return err
	}
	build, _ := cmd.Flags().GetString("build")
	if build == "" {
		return errReleaseBuildRequired()
	}
	version, _ := cmd.Flags().GetString("policy-version")
	requestID, _ := cmd.Flags().GetString("request-id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	if parsed, parseErr := uuid.Parse(requestID); parseErr != nil || parsed == uuid.Nil || parsed.String() != requestID {
		return errReleaseRequestID()
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	fetched := version == ""
	// A resume reads the policy even when --policy-version was given, because
	// the printed hint supplies both and the landed check needs the current
	// version. The explicit version is still what the request asserts.
	if resuming := cmd.Flags().Changed("request-id"); fetched || resuming {
		var policy releasePolicy
		if _, err = apiCall(cmd.Context(), http.MethodGet, releaseURL(id, "auto-deploy"), nil, at.Token, verbose, &policy); err != nil {
			return err
		}
		// A retry sets the policy version to its request ID, so finding them
		// equal means this request already landed.
		if policy.Version == requestID {
			if policy.DesiredBuild != build {
				return errReleaseRetryBuildMismatch(requestID, policy.DesiredBuild, build)
			}
			if output, _ := cmd.Flags().GetBool("json"); output {
				return writeJSON(cmd.OutOrStdout(), policy)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), msgReleaseRetryAlreadyApplied(id, requestID))
			return err
		}
		if fetched {
			version = policy.Version
		}
	}
	if parsed, parseErr := uuid.Parse(version); parseErr != nil || parsed == uuid.Nil || parsed.String() != version || version == requestID {
		if fetched {
			return errReleaseNoPolicyToRetry(id)
		}
		return errReleasePolicyVersion()
	}
	body := map[string]string{"policy_version": version, "build_id": build, "request_id": requestID}
	var result releasePolicy
	status, err := apiCall(cmd.Context(), http.MethodPost, releaseURL(id, "auto-deploy", "retry"), body, at.Token, verbose, &result)
	if err != nil {
		// Resuming only helps when the response was lost. A rejection repeats.
		// A 2xx that failed to decode landed, so it needs the hint as well.
		if status < 0 || status >= 500 || status == http.StatusTooManyRequests || (status >= 200 && status < 300) {
			return errReleaseRetryRequest(version, requestID, err)
		}
		return err
	}
	if output, _ := cmd.Flags().GetBool("json"); output {
		return writeJSON(cmd.OutOrStdout(), result)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgReleaseRetry(result))
	return err
}
