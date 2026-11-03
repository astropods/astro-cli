package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

const releasePayloadLimit = 4 << 20

type releasePlan struct {
	CandidateID string          `json:"candidate_id"`
	TargetID    string          `json:"target_id"`
	BuildID     string          `json:"build_id"`
	Revision    int             `json:"revision"`
	Policy      json.RawMessage `json:"policy"`
	Version     string          `json:"version"`
}

func newReleaseSetupCommand() *cobra.Command {
	command := &cobra.Command{Use: "setup", Short: "Validate and prepare evaluation-gated automatic deployment", Long: msgReleaseSetupHelp(), Args: cobra.NoArgs, RunE: runReleaseSetup}
	command.Flags().String("id", "", "Target deployment ID")
	command.Flags().String("candidate", "", "Candidate deployment ID")
	command.Flags().String("dataset", "", "Dataset JSON file")
	command.Flags().String("rules", "", "Acceptance rules JSON array")
	command.Flags().StringSlice("runner", nil, "Trusted app:ID or user:ID; repeat for additional identities")
	command.Flags().Int("max-age", 300, "Maximum evidence age in seconds, from 1 to 3600")
	command.Flags().String("output", "promotion-plan.json", "File to write the validated plan")
	command.Flags().Bool("apply", false, "Create the policy and enable candidate and target automatic deployment")
	command.Flags().Bool("json", false, "Print the validated plan as JSON")
	return command
}

func readReleaseJSON(path string) (json.RawMessage, error) {
	file, err := os.Open(path) // #nosec G304 -- path is an explicit CLI file argument.
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck
	raw, err := io.ReadAll(io.LimitReader(file, releasePayloadLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > releasePayloadLimit || !json.Valid(raw) {
		return nil, errReleaseJSONFile(path)
	}
	return raw, nil
}

func runReleaseSetup(cmd *cobra.Command, _ []string) error {
	target, err := releaseDeploymentID(cmd)
	if err != nil {
		return err
	}
	candidate, _ := cmd.Flags().GetString("candidate")
	datasetPath, _ := cmd.Flags().GetString("dataset")
	rulesPath, _ := cmd.Flags().GetString("rules")
	runners, _ := cmd.Flags().GetStringSlice("runner")
	maxAge, _ := cmd.Flags().GetInt("max-age")
	output, _ := cmd.Flags().GetString("output")
	if candidate == "" || candidate == target || datasetPath == "" || rulesPath == "" || len(runners) == 0 || maxAge < 1 || maxAge > 3600 || output == "" {
		return errReleaseSetupFlags()
	}
	dataset, err := readReleaseJSON(datasetPath)
	if err != nil {
		return err
	}
	rules, err := readReleaseJSON(rulesPath)
	if err != nil {
		return err
	}
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	body := map[string]any{"target_id": target, "dataset": dataset, "rules": rules, "trusted_runners": runners, "max_age_seconds": maxAge}
	var plan releasePlan
	if _, err = apiCall(cmd.Context(), http.MethodPost, releaseURL(candidate, "build-evaluation-preflight"), body, at.Token, verbose, &plan); err != nil {
		return err
	}
	if plan.CandidateID != candidate || plan.TargetID != target || plan.Version == "" || !json.Valid(plan.Policy) {
		return errReleaseSetupResponse()
	}
	raw, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	if _, err = writeFileAtomically(output, bytes.NewReader(append(raw, '\n'))); err != nil {
		return err
	}
	apply, _ := cmd.Flags().GetBool("apply")
	if apply {
		var existing releasePolicy
		if _, err = apiCall(cmd.Context(), http.MethodGet, releaseURL(candidate, "auto-deploy"), nil, at.Token, verbose, &existing); err != nil {
			return err
		}
		if existing.Mode == "accepted" {
			return errReleaseCandidatePolicy()
		}
		candidatePolicy := map[string]any{"enabled": true, "mode": "published"}
		targetPolicy := map[string]any{"enabled": true, "mode": "accepted", "candidate_id": candidate, "acceptance_version": plan.Version}
		for _, check := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, releaseURL(candidate, "build-acceptance-policies"), plan.Policy},
			{http.MethodPut, releaseURL(candidate, "auto-deploy"), candidatePolicy},
			{http.MethodPut, releaseURL(target, "auto-deploy"), targetPolicy},
		} {
			if _, err = apiCall(cmd.Context(), check.method, check.path+"?dryrun=true", check.body, at.Token, verbose, nil); err != nil {
				return err
			}
		}
		var accepted struct {
			Version string `json:"version"`
		}
		if _, err = apiCall(cmd.Context(), http.MethodPost, releaseURL(candidate, "build-acceptance-policies"), plan.Policy, at.Token, verbose, &accepted); err != nil {
			return err
		}
		if accepted.Version != plan.Version {
			return errReleaseSetupResponse()
		}
		var policy releasePolicy
		if _, err = apiCall(cmd.Context(), http.MethodPut, releaseURL(candidate, "auto-deploy"), candidatePolicy, at.Token, verbose, &policy); err != nil {
			return err
		}
		if _, err = apiCall(cmd.Context(), http.MethodPut, releaseURL(target, "auto-deploy"), targetPolicy, at.Token, verbose, &policy); err != nil {
			return errReleaseSetupPartial(err)
		}
	}
	if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
		return writeJSON(cmd.OutOrStdout(), plan)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), msgReleaseSetupComplete(output, apply))
	return err
}
