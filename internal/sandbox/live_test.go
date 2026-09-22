//go:build live

package sandbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

// Runs against real Docker. Needs astro-sandbox:local (moon run
// astro-sandbox:image-local) and the network below.
//
//	docker network create astro-sbx-live
//	go test -tags live ./internal/sandbox/ -run Live -v
func TestLiveEnsureServesAnExecAfterTheRunHook(t *testing.T) {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	r := NewRunner(cli, Config{
		Image:   "astro-sandbox:local",
		Network: "astro-sbx-live",
		Project: "live-test",
	})
	ctx := context.Background()
	t.Cleanup(func() { _ = r.RemoveAll(ctx) })

	inst, err := r.Ensure(ctx, "conv-live-1", "tok-live")
	require.NoError(t, err, "Ensure must bring a real sandbox up and install the token")
	require.True(t, inst.Started)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.ControlURL+"/v1/exec",
		strings.NewReader(`{"command":["/bin/sh","-c","echo live-ok"]}`))
	require.NoError(t, err)
	req.Header.Set("X-Access-Token", "tok-live")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, res.StatusCode, "the token the run hook installed must authorize an exec")
}

// TestLiveTheBrokerServesAnAgentsAttachAndExec drives the whole local path: a
// signed token, attach over the control-plane route, then an exec against the
// handle the agent would receive.
func TestLiveTheBrokerServesAnAgentsAttachAndExec(t *testing.T) {
	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)

	runner := NewRunner(cli, Config{
		Image:   "astro-sandbox:local",
		Network: "astro-sbx-live",
		Project: "live-broker",
	})
	ctx := context.Background()
	t.Cleanup(func() { _ = runner.RemoveAll(ctx) })

	secret, err := NewSecret()
	require.NoError(t, err)
	broker := httptest.NewServer(NewServer(runner, secret, slog.New(slog.DiscardHandler)).Handler())
	t.Cleanup(broker.Close)

	agentToken, err := SignToken("local", broker.URL, secret)
	require.NoError(t, err)

	attach, err := http.NewRequestWithContext(ctx, http.MethodPut, broker.URL+"/api/v1/sandboxes/conv-broker", nil)
	require.NoError(t, err)
	attach.Header.Set("Authorization", "Bearer "+agentToken)

	res, err := http.DefaultClient.Do(attach)
	require.NoError(t, err)
	defer res.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, res.StatusCode)

	var handle HandleResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&handle))
	require.NotEmpty(t, handle.Headers[sessionTokenHeader])

	// The handle's endpoint is in-network, which this test cannot resolve from
	// the host, so the exec goes over the runner's loopback route with the
	// token the handle carried.
	inst, err := runner.Ensure(ctx, "conv-broker", handle.Headers[sessionTokenHeader])
	require.NoError(t, err)

	exec, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.ControlURL+"/v1/exec",
		strings.NewReader(`{"command":["/bin/sh","-c","echo broker-ok"]}`))
	require.NoError(t, err)
	exec.Header.Set(sessionTokenHeader, handle.Headers[sessionTokenHeader])

	execRes, err := http.DefaultClient.Do(exec)
	require.NoError(t, err)
	defer execRes.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, execRes.StatusCode,
		"the token the handle carried must authorize the data plane the handle points at")
}
