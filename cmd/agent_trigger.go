package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/spf13/cobra"
)

const ingestionComponentPrefix = "ingestion-"

var agentTriggerCmd = &cobra.Command{
	Use:   "trigger [job]",
	Short: "Run a deployed ingestion job now",
	Long: `Run a deployed ingestion job now, without waiting for its schedule.

Omit the job name to list the ingestion jobs on the deployment.`,
	Args: agentTriggerArgs,
	RunE: runAgentTrigger,
}

func agentTriggerArgs(_ *cobra.Command, args []string) error {
	if len(args) > 1 {
		return errAgentUnexpectedArgument(args[1])
	}
	return nil
}

// ingestionJob is a deployed ingestion workload, keyed by the name the trigger
// endpoint expects rather than the workload name.
type ingestionJob struct {
	name     string
	schedule string
}

func ingestionJobsFromWorkloads(workloads []workloadDetail) []ingestionJob {
	jobs := []ingestionJob{}
	for _, wl := range workloads {
		if !strings.HasPrefix(wl.Component, ingestionComponentPrefix) {
			continue
		}
		jobs = append(jobs, ingestionJob{
			name:     strings.TrimPrefix(wl.Component, ingestionComponentPrefix),
			schedule: wl.Schedule,
		})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].name < jobs[j].name })
	return jobs
}

func ingestionJobNames(jobs []ingestionJob) []string {
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		names = append(names, j.name)
	}
	return names
}

func runAgentTrigger(cmd *cobra.Command, args []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}
	label := deploymentLabel(dep)

	detail, err := getDeploymentDetail(cmd, dep.ID, at, verbose)
	if err != nil {
		return fmt.Errorf("fetching deployment detail: %w", err)
	}

	jobs := ingestionJobsFromWorkloads(detail.Workloads)
	if len(jobs) == 0 {
		return errAgentNoIngestionJobs(label)
	}

	w := cmd.OutOrStdout()

	if len(args) == 0 {
		fmt.Fprintf(w, "%s\n\n", msgAgentIngestionJobsHeader(label)) //nolint:errcheck,gosec
		for _, j := range jobs {
			cadence := ""
			if j.schedule != "" {
				cadence = fmt.Sprintf("  %s(%s)%s", colorDim, j.schedule, colorReset)
			}
			fmt.Fprintf(w, "  %s%s%s%s\n", colorBold, j.name, colorReset, cadence) //nolint:errcheck,gosec
		}
		fmt.Fprintf(w, "\nRun %s%s agent trigger <job>%s to run one now.\n", colorBold, buildinfo.BinaryName, colorReset) //nolint:errcheck,gosec
		return nil
	}

	name := args[0]
	found := false
	for _, j := range jobs {
		if j.name == name {
			found = true
			break
		}
	}
	if !found {
		return errAgentUnknownIngestionJob(name, ingestionJobNames(jobs))
	}

	fmt.Fprintf(w, "%s→%s %s\n", colorCyan, colorReset, msgAgentTriggering(name, label)) //nolint:errcheck,gosec

	u := fmt.Sprintf("%s/api/v1/deployments/%s/ingestion/%s/trigger",
		agentBaseURL(), url.PathEscape(dep.ID), url.PathEscape(name))

	status, err := apiCall(cmd.Context(), http.MethodPost, u, nil, at.Token, verbose, nil)
	if status == http.StatusNotFound {
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "  %s✓%s %s\n", colorGreen, colorReset, msgAgentTriggered(name)) //nolint:errcheck,gosec
	return nil
}
