package agentcore

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Target selects where the agent runs. "aws" creates a real runtime; "local"
// skips all AWS calls and emits the localhost wiring (the `ast dev` shape).
type Target string

const (
	TargetAWS   Target = "aws"
	TargetLocal Target = "local"
)

// Runtime is the control-plane seam. GetByName resolves an existing runtime ARN
// by name (empty arn if absent); Create and Update mutate. The AWS-CLI impl and
// test fakes both satisfy it, so the deploy logic is testable without AWS.
type Runtime interface {
	GetByName(name string) (arn, id string, err error)
	Create(req CreateAgentRuntime, name, region string) (arn, id, version string, err error)
	Update(id string, req CreateAgentRuntime, region string) (arn, version string, err error)
}

// Result is what a deploy produced: the resolved identifiers plus the env block
// the messaging service must run with to talk to this agent.
type Result struct {
	Target       Target            `json:"target"`
	RuntimeName  string            `json:"runtimeName,omitempty"`
	RuntimeArn   string            `json:"runtimeArn,omitempty"`
	RuntimeID    string            `json:"runtimeId,omitempty"`
	Version      string            `json:"version,omitempty"`
	Action       string            `json:"action"` // "created" | "updated" | "local"
	MessagingEnv map[string]string `json:"messagingEnv"`
}

// Run executes the deploy for the given plan. For TargetLocal it makes no AWS
// calls and returns the localhost wiring; for TargetAWS it create-or-updates the
// runtime via rt and returns the resolved ARN + messaging env.
//
// localEndpoint is the agent's base URL for local mode (e.g. http://localhost:8080).
func Run(p *Plan, target Target, rt Runtime, region, localEndpoint string) (*Result, error) {
	switch target {
	case TargetLocal:
		return &Result{
			Target:      TargetLocal,
			RuntimeName: p.AgentCore.AgentRuntimeName,
			Action:      "local",
			MessagingEnv: map[string]string{
				"AGENT_TRANSPORT":        "agentcore",
				"ASTRO_DEPLOY_TARGET":    "local",
				"AGENT_RUNTIME_ENDPOINT": orStr(localEndpoint, "http://localhost:8080"),
			},
		}, nil

	case TargetAWS:
		if rt == nil {
			return nil, fmt.Errorf("deploy aws: no Runtime backend configured")
		}
		name := p.AgentCore.AgentRuntimeName
		existingArn, existingID, err := rt.GetByName(name)
		if err != nil {
			return nil, fmt.Errorf("resolve existing runtime %q: %w", name, err)
		}

		var arn, id, version, action string
		if existingArn == "" {
			arn, id, version, err = rt.Create(p.AgentCore, name, region)
			action = "created"
		} else {
			// Idempotent re-deploy: update the existing runtime in place rather
			// than spawning a parallel one (avoids version sprawl).
			arn, version, err = rt.Update(existingID, p.AgentCore, region)
			id = existingID
			action = "updated"
		}
		if err != nil {
			return nil, fmt.Errorf("%s runtime %q: %w", action, name, err)
		}

		return &Result{
			Target:      TargetAWS,
			RuntimeName: name,
			RuntimeArn:  arn,
			RuntimeID:   id,
			Version:     version,
			Action:      action,
			MessagingEnv: map[string]string{
				"AGENT_TRANSPORT":     "agentcore",
				"ASTRO_DEPLOY_TARGET": "aws",
				"AGENT_RUNTIME_ARN":   arn,
				"AWS_REGION":          region,
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown deploy target %q (want aws|local)", target)
	}
}

// EnvExports renders MessagingEnv as copy-pasteable `KEY=value` lines (sorted),
// so the operator can feed the messaging sidecar's Deployment patch.
func (r *Result) EnvExports() string {
	keys := make([]string, 0, len(r.MessagingEnv))
	for k := range r.MessagingEnv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, r.MessagingEnv[k])
	}
	return b.String()
}

// JSON renders the full result.
func (r *Result) JSON() (string, error) {
	out, err := json.MarshalIndent(r, "", "  ")
	return string(out), err
}
