package compose

import (
	"io"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
)

// BuildEnvironment is an allowlist: it copies named keys out of envVars rather
// than passing the map through. A variable the agent needs and this function
// does not name is dropped without a word.
func TestTheSandboxTokenReachesTheAgent(t *testing.T) {
	s := &spec.AstroSpec{Name: "my-agent", Agent: spec.Container{Image: "x"}}

	env := BuildEnvironment(s, map[string]string{
		"ASTRO_AUTHZ_TOKEN": "a.dev.session.token",
	})

	token, ok := env["ASTRO_AUTHZ_TOKEN"]
	if !ok || token == nil {
		t.Fatal("the agent SDK reads ASTRO_AUTHZ_TOKEN, and without it the agent runs with authorization off")
	}
	assert.Equal(t, "a.dev.session.token", *token)
}

func TestNoSandboxTokenMeansNoVariable(t *testing.T) {
	s := &spec.AstroSpec{Name: "my-agent", Agent: spec.Container{Image: "x"}}

	env := BuildEnvironment(s, map[string]string{})

	assert.NotContains(t, env, "ASTRO_AUTHZ_TOKEN",
		"an empty value would look like a credential to the SDK and fail every call")
}

// The messaging sidecar uses ASTRO_AUTHZ_TOKEN as a deployment identity, for
// the per-request authorization callback. A dev session token names a session,
// not a deployment, and the control plane's deploy-token middleware refuses a
// token that carries a kind. Handing it to the sidecar would send a credential
// to an endpoint that must reject it.
func TestTheSandboxTokenDoesNotReachTheMessagingSidecar(t *testing.T) {
	s := &spec.AstroSpec{Name: "my-agent", Agent: spec.Container{Image: "x"}}

	env := buildMessagingEnvironment(s, map[string]string{
		"ASTRO_AUTHZ_TOKEN": "a.dev.session.token",
	}, io.Discard)

	assert.NotContains(t, env, "ASTRO_AUTHZ_TOKEN",
		"a dev session is not a deployment, so the sidecar's authorize callback would be refused")
}
