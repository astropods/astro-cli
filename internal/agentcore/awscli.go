package agentcore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// AWSCLIRuntime drives the bedrock-agentcore-control API via the local, already
// authenticated `aws` CLI. This keeps the CLI free of the AWS SDK dependency
// tree; the `aws` CLI resolves the same credential chain (profile / env / role).
//
// NetworkMode note: the network mode comes from the plan (PUBLIC for the no-EKS
// POC, invoke over SigV4 from anywhere with creds; VPC when the plan carries
// subnets). networkJSON reads plan.NetworkMode so the emitted command always
// matches the plan.
type AWSCLIRuntime struct {
	Profile string
	Region  string
	// DryRun prints the commands instead of executing them (for --dry-run).
	DryRun bool
	// SecretKeys are env var names whose values must be masked in the printed
	// (dry-run) command so secrets never land in logs. The real values are still
	// sent on an actual (non-dry-run) call.
	SecretKeys map[string]bool
	// Out, when set, receives each dry-run command line (so the caller can route
	// it through cmd.OutOrStdout()); falls back to fmt.Println when nil.
	Out func(string)
}

func (a *AWSCLIRuntime) base() []string {
	args := []string{"bedrock-agentcore-control"}
	if a.Region != "" {
		args = append(args, "--region", a.Region)
	}
	if a.Profile != "" {
		args = append(args, "--profile", a.Profile)
	}
	return args
}

// GetByName lists runtimes and finds one matching name. Empty arn ⇒ not found.
func (a *AWSCLIRuntime) GetByName(name string) (arn, id string, err error) {
	out, err := a.run(append(a.base(), "list-agent-runtimes", "--max-results", "100"))
	if err != nil {
		return "", "", err
	}
	var resp struct {
		AgentRuntimes []struct {
			AgentRuntimeArn  string `json:"agentRuntimeArn"`
			AgentRuntimeID   string `json:"agentRuntimeId"`
			AgentRuntimeName string `json:"agentRuntimeName"`
		} `json:"agentRuntimes"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("parse list-agent-runtimes: %w", err)
	}
	for _, r := range resp.AgentRuntimes {
		if r.AgentRuntimeName == name {
			return r.AgentRuntimeArn, r.AgentRuntimeID, nil
		}
	}
	return "", "", nil
}

// Create issues create-agent-runtime and returns the new ARN/id/version.
func (a *AWSCLIRuntime) Create(req CreateAgentRuntime, name, region string) (arn, id, version string, err error) {
	args := append(a.base(),
		"create-agent-runtime",
		"--agent-runtime-name", name,
		"--agent-runtime-artifact", artifactJSON(req),
		"--role-arn", req.RoleArn,
		"--network-configuration", a.networkJSON(req),
		"--protocol-configuration", `{"serverProtocol":"HTTP"}`,
		"--lifecycle-configuration", lifecycleJSON(req),
		"--environment-variables", envJSON(req.Env),
	)
	out, err := a.run(args)
	if err != nil {
		return "", "", "", err
	}
	if a.DryRun {
		return "arn:aws:bedrock-agentcore:" + region + ":DRYRUN:runtime/" + name, name + "-DRYRUN", "1", nil
	}
	var resp struct {
		AgentRuntimeArn     string `json:"agentRuntimeArn"`
		AgentRuntimeID      string `json:"agentRuntimeId"`
		AgentRuntimeVersion string `json:"agentRuntimeVersion"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", "", fmt.Errorf("parse create-agent-runtime: %w", err)
	}
	return resp.AgentRuntimeArn, resp.AgentRuntimeID, resp.AgentRuntimeVersion, nil
}

// Update issues update-agent-runtime for an existing runtime id.
func (a *AWSCLIRuntime) Update(id string, req CreateAgentRuntime, region string) (arn, version string, err error) {
	args := append(a.base(),
		"update-agent-runtime",
		"--agent-runtime-id", id,
		"--agent-runtime-artifact", artifactJSON(req),
		"--role-arn", req.RoleArn,
		"--network-configuration", a.networkJSON(req),
		"--protocol-configuration", `{"serverProtocol":"HTTP"}`,
		"--lifecycle-configuration", lifecycleJSON(req),
		"--environment-variables", envJSON(req.Env),
	)
	out, err := a.run(args)
	if err != nil {
		return "", "", err
	}
	if a.DryRun {
		return "arn:aws:bedrock-agentcore:" + region + ":DRYRUN:runtime/" + id, "N", nil
	}
	var resp struct {
		AgentRuntimeArn     string `json:"agentRuntimeArn"`
		AgentRuntimeVersion string `json:"agentRuntimeVersion"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", "", fmt.Errorf("parse update-agent-runtime: %w", err)
	}
	return resp.AgentRuntimeArn, resp.AgentRuntimeVersion, nil
}

// Status reads one runtime's lifecycle state, and why it failed when it did.
func (a *AWSCLIRuntime) Status(id string) (RuntimeStatus, error) {
	out, err := a.run(append(a.base(), "get-agent-runtime", "--agent-runtime-id", id))
	if err != nil {
		return RuntimeStatus{}, err
	}
	var resp struct {
		Status              string `json:"status"`
		AgentRuntimeVersion string `json:"agentRuntimeVersion"`
		FailureReason       string `json:"failureReason"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return RuntimeStatus{}, fmt.Errorf("parse get-agent-runtime: %w", err)
	}
	return RuntimeStatus{Status: resp.Status, Version: resp.AgentRuntimeVersion, FailureReason: resp.FailureReason}, nil
}

func (a *AWSCLIRuntime) networkJSON(req CreateAgentRuntime) string {
	if req.NetworkMode != "VPC" {
		return `{"networkMode":"PUBLIC"}`
	}
	nc := map[string]any{
		"networkMode": "VPC",
		"networkModeConfig": map[string]any{
			"subnets":        req.NetworkConfig.Subnets,
			"securityGroups": req.NetworkConfig.SecurityGroups,
		},
	}
	b, _ := json.Marshal(nc)
	return string(b)
}

func lifecycleJSON(req CreateAgentRuntime) string {
	b, _ := json.Marshal(req.Lifecycle)
	return string(b)
}

func artifactJSON(req CreateAgentRuntime) string {
	b, _ := json.Marshal(map[string]any{
		"containerConfiguration": map[string]string{"containerUri": req.Container.ImageURI},
	})
	return string(b)
}

func envJSON(env map[string]string) string {
	b, _ := json.Marshal(env)
	return string(b)
}

// maskEnvArg returns a copy of args with the value of the
// `--environment-variables` argument re-rendered so any SecretKeys value is
// replaced by "***". Used only for the dry-run printout.
func (a *AWSCLIRuntime) maskEnvArg(args []string) []string {
	if len(a.SecretKeys) == 0 {
		return args
	}
	out := append([]string(nil), args...)
	for i := 0; i+1 < len(out); i++ {
		if out[i] != "--environment-variables" {
			continue
		}
		var env map[string]string
		if json.Unmarshal([]byte(out[i+1]), &env) != nil {
			continue
		}
		for k := range env {
			if a.SecretKeys[k] {
				env[k] = "***"
			}
		}
		out[i+1] = envJSON(env)
	}
	return out
}

// run executes (or, in DryRun, prints) an aws CLI invocation.
func (a *AWSCLIRuntime) run(args []string) ([]byte, error) {
	if a.DryRun {
		line := "+ aws " + strings.Join(quoteArgs(a.maskEnvArg(args)), " ")
		if a.Out != nil {
			a.Out(line)
		} else {
			fmt.Println(line)
		}
		return []byte("{}"), nil
	}
	cmd := exec.Command("aws", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		return nil, fmt.Errorf("aws %s: %v: %s%s", args[0], err, msg, staleAWSCLIHint(msg))
	}
	return stdout.Bytes(), nil
}

// quoteArgs wraps args containing spaces/braces in single quotes for display.
func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " {}\"") {
			out[i] = "'" + a + "'"
		} else {
			out[i] = a
		}
	}
	return out
}

// staleAWSCLIHint names the cause when the installed aws CLI's bundled service
// model predates VPC network mode, which fails local validation, not the API.
func staleAWSCLIHint(stderr string) string {
	if !strings.Contains(stderr, "networkModeConfig") {
		return ""
	}
	return "\n\nThe installed aws CLI does not know VPC network mode. Upgrade it (2.36.21 is known good): brew upgrade awscli"
}
