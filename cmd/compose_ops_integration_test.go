//go:build integration

package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJobComposeService_OneOffContainerOutputReachesWriters(t *testing.T) {
	probe, err := command.NewDockerCli()
	require.NoError(t, err)
	if err := probe.Initialize(cliflags.NewClientOptions()); err != nil {
		t.Skipf("docker unavailable: %v", err)
	}
	if _, err := probe.Client().Ping(t.Context(), client.PingOptions{}); err != nil {
		t.Skipf("docker daemon unreachable: %v", err)
	}

	var out, errOut bytes.Buffer
	svc, err := newJobComposeService(&out, &errOut, false)
	require.NoError(t, err)

	project := &types.Project{
		Name: "astro-it-job-output",
		Services: types.Services{
			"job": {
				Name:    "job",
				Image:   "docker.io/library/busybox:1.36",
				Command: types.ShellCommand{"sh", "-c", "echo job-stdout-marker; echo job-stderr-marker >&2"},
			},
		},
	}
	exitCode, err := svc.RunOneOffContainer(context.Background(), project, api.RunOptions{
		Service:    "job",
		AutoRemove: true,
		NoDeps:     true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode)

	assert.Contains(t, out.String(), "job-stdout-marker", "a job's stdout must reach the writer, not be discarded")
	assert.Contains(t, errOut.String(), "job-stderr-marker", "a job's stderr must reach the error writer")
}
