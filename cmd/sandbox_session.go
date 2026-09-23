package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-spec"
)

// sandboxServerURLOverride is set in tests to redirect API calls to a test
// server.
var sandboxServerURLOverride string

func sandboxBaseURL() string {
	if sandboxServerURLOverride != "" {
		return strings.TrimSuffix(sandboxServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// sandboxTokenEnvVar is what the agent SDK reads to find the control plane and
// authorize against it. The SDK takes the base URL from the token's own `iss`
// claim, so nothing else has to be injected.
// #nosec G101 -- the name of an environment variable, not a credential.
const sandboxTokenEnvVar = "ASTRO_AUTHZ_TOKEN"

type openDevSessionRequest struct {
	AgentName string `json:"agent_name"`
}

type openDevSessionResponse struct {
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// injectSandboxDevToken opens a dev session for this agent and puts its token
// in envVars, so an agent running locally attaches a real sandbox through the
// deployed control plane. There is no local sandbox runtime: the agent talks to
// the same MicroVM it would when deployed.
//
// A no-op unless asked for, because the spec has no field to gate on yet and
// `ast dev` must keep working for an author with no interest in sandboxes.
func injectSandboxDevToken(
	ctx context.Context,
	w io.Writer,
	agentName string,
	envVars map[string]string,
	wanted, verbose bool,
) error {
	if !wanted {
		return nil
	}
	if agentName == "" {
		return errSandboxNeedsAgentName()
	}

	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return errSandboxRequiresLogin(err)
	}

	u := apiPath(sandboxBaseURL(), at.Account, "accounts", "dev-sessions")
	var resp openDevSessionResponse
	status, err := apiCall(ctx, http.MethodPost, u,
		openDevSessionRequest{AgentName: agentName}, at.Token, verbose, &resp)
	if err != nil {
		if status == http.StatusConflict {
			return errSandboxNotEnabled(at.Account)
		}
		return errSandboxSessionFailed(err)
	}
	if resp.Token == "" {
		return errSandboxSessionFailed(fmt.Errorf("the server returned no token"))
	}

	envVars[sandboxTokenEnvVar] = resp.Token
	fmt.Fprintf(w, "%s→%s %s\n", colorCyan, colorReset, msgSandboxSessionOpened(resp.ExpiresAt)) //nolint:errcheck,gosec
	return nil
}

// closeSandboxDevSession revokes the session and drops its sandboxes, so a
// developer who has finished stops paying for compute. Failures warn rather
// than abort: teardown has other work to finish, and the session expires and
// is swept regardless.
func closeSandboxDevSession(ctx context.Context, w io.Writer, agentName string, verbose bool) {
	if agentName == "" {
		return
	}
	at, err := getCurrentAccountToken(ctx)
	if err != nil {
		return
	}

	u := apiPath(sandboxBaseURL(), at.Account, "accounts", "dev-sessions", url.PathEscape(agentName))
	if _, err := apiCall(ctx, http.MethodDelete, u, nil, at.Token, verbose, nil); err != nil {
		fmt.Fprintf(w, "%s!%s %s%s%s\n", colorYellow, colorReset, colorDim, msgSandboxSessionCloseFailed(err), colorReset) //nolint:errcheck,gosec
	}
}

// closeSandboxSessionForProject closes the session for the agent in the
// current directory. `project stop` does not know whether a sandbox was asked
// for, and closing one that does not exist is a no-op, so it always tries.
func closeSandboxSessionForProject(cmd *cobra.Command) {
	specPath, err := resolveSpecPathFromCwd("")
	if err != nil {
		return
	}
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil || astroSpec == nil {
		return
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	closeSandboxDevSession(cmd.Context(), cmd.OutOrStdout(), astroSpec.Name, verbose)
}

// declaresSandbox reports whether the spec asks for a sandbox. The section's
// presence is the request: astro-server refuses an attach from a deployment
// whose blueprint declares none, and RFC-1 section 9 requires a toolchain
// inside the section so it cannot be an empty marker.
func declaresSandbox(s *spec.AstroSpec) bool {
	return s != nil && s.Sandbox != nil
}
