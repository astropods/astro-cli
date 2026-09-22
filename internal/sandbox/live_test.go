//go:build live

package sandbox

import (
	"context"
	"net/http"
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
