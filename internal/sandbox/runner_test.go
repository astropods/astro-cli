package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real client must satisfy the narrow interface, or the runner cannot be
// handed the client `ast dev` already builds.
var _ Docker = (*dockerclient.Client)(nil)

type fakeDocker struct {
	list []container.Summary

	created     int
	createdName string
	createdCfg  *container.Config
	createdHost *container.HostConfig
	createdNet  *network.NetworkingConfig
	started     int
	removed     []string

	createErr error
	removeErr error
	noPorts   bool
}

func (f *fakeDocker) ContainerCreate(_ context.Context, cfg *container.Config, hostCfg *container.HostConfig,
	netCfg *network.NetworkingConfig, _ *ocispec.Platform, name string,
) (container.CreateResponse, error) {
	if f.createErr != nil {
		return container.CreateResponse{}, f.createErr
	}
	f.created++
	f.createdName = name
	f.createdCfg = cfg
	f.createdHost = hostCfg
	f.createdNet = netCfg
	return container.CreateResponse{ID: "container-" + name}, nil
}

func (f *fakeDocker) ContainerStart(context.Context, string, container.StartOptions) error {
	f.started++
	return nil
}

func (f *fakeDocker) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	if f.noPorts {
		return container.InspectResponse{NetworkSettings: &container.NetworkSettings{}}, nil
	}
	return container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			NetworkSettingsBase: container.NetworkSettingsBase{
				Ports: nat.PortMap{
					dataPlanePortSpec: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "34001"}},
					hookPortSpec:      []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "34002"}},
				},
			},
		},
	}, nil
}

func (f *fakeDocker) ContainerList(context.Context, container.ListOptions) ([]container.Summary, error) {
	return f.list, nil
}

func (f *fakeDocker) ContainerRemove(_ context.Context, id string, _ container.RemoveOptions) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, id)
	return nil
}

type recordedRequest struct {
	method string
	url    string
	body   string
}

type fakeGuest struct {
	requests []recordedRequest

	healthStatus int
	hookStatus   int
	healthErr    error
}

func (g *fakeGuest) Do(req *http.Request) (*http.Response, error) {
	var body string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	g.requests = append(g.requests, recordedRequest{method: req.Method, url: req.URL.String(), body: body})

	reply := func(status int) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	}

	if strings.HasSuffix(req.URL.Path, "/healthz") {
		if g.healthErr != nil {
			return nil, g.healthErr
		}
		if g.healthStatus != 0 {
			return reply(g.healthStatus)
		}
		return reply(http.StatusOK)
	}
	if g.hookStatus != 0 {
		return reply(g.hookStatus)
	}
	return reply(http.StatusOK)
}

func newRunner(t *testing.T) (*Runner, *fakeDocker, *fakeGuest) {
	t.Helper()
	docker := &fakeDocker{}
	guest := &fakeGuest{}
	r := NewRunner(docker, Config{
		Image:   "astropods/sandbox:test",
		Network: "agent-network",
		Project: "my-agent",
	})
	r.http = guest
	r.readyInterval = 0
	return r, docker, guest
}

func runningSummary(name string) container.Summary {
	return container.Summary{
		ID:     "existing-1",
		State:  "running",
		Labels: map[string]string{labelName: name, labelProject: "my-agent"},
	}
}

func TestEnsureCreatesASandboxAndInstallsTheTokenThroughTheRunHook(t *testing.T) {
	r, docker, guest := newRunner(t)

	inst, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.NoError(t, err)

	assert.Equal(t, 1, docker.created)
	assert.Equal(t, 1, docker.started)
	assert.True(t, inst.Started)
	assert.Equal(t, "astropods/sandbox:test", docker.createdCfg.Image)

	require.Len(t, guest.requests, 2, "the runner probes health, then posts the run hook")
	assert.Contains(t, guest.requests[0].url, "/healthz")

	hook := guest.requests[1]
	assert.Equal(t, http.MethodPost, hook.method)
	assert.Contains(t, hook.url, RuntimePrefix+"/run",
		"the run hook must use the path the platform uses, so the guest serves one path")
	assert.NotContains(t, hook.url, "34001",
		"the hook server is a separate port from the data plane, and no agent credential reaches it")

	var envelope map[string]string
	require.NoError(t, json.Unmarshal([]byte(hook.body), &envelope))
	assert.Contains(t, envelope, "microvmId")
	require.Contains(t, envelope, "runHookPayload",
		"the platform wraps the payload as a string under this key")

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(envelope["runHookPayload"]), &payload))
	assert.Equal(t, "session-token", payload["envd_token"])
	assert.Equal(t, "conv-1", payload["sandbox_id"])
}

func TestEnsureReusesARunningSandboxForTheSameName(t *testing.T) {
	r, docker, guest := newRunner(t)
	docker.list = []container.Summary{runningSummary("conv-1")}

	inst, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.NoError(t, err)

	assert.Zero(t, docker.created, "a second attach on one name must not start a second sandbox")
	assert.False(t, inst.Started)
	assert.Empty(t, guest.requests, "reuse must not reinstall the token, which would break a live session")
	assert.Equal(t, "existing-1", inst.ContainerID)
}

func TestEnsureReplacesAStoppedSandbox(t *testing.T) {
	r, docker, _ := newRunner(t)
	stopped := runningSummary("conv-1")
	stopped.State = "exited"
	docker.list = []container.Summary{stopped}

	_, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.NoError(t, err)

	assert.Equal(t, []string{"existing-1"}, docker.removed,
		"a stopped sandbox lost the token the run hook installed, so it cannot be reused")
	assert.Equal(t, 1, docker.created)
}

func TestEnsureJoinsTheAgentNetworkUnderItsOwnAlias(t *testing.T) {
	r, docker, _ := newRunner(t)

	inst, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.NoError(t, err)

	endpoints := docker.createdNet.EndpointsConfig
	require.Contains(t, endpoints, "agent-network",
		"the sandbox must join the agent's network, or the agent cannot reach it")
	assert.Equal(t, []string{inst.Host}, endpoints["agent-network"].Aliases)
	assert.Equal(t, "http://"+inst.Host+":49983", inst.Endpoint,
		"the endpoint must be in-network, so exec allocates no host port")
}

func TestThePortsAreBoundToLoopbackOnly(t *testing.T) {
	r, docker, _ := newRunner(t)

	_, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.NoError(t, err)

	bindings := docker.createdHost.PortBindings
	require.Contains(t, bindings, hookPortSpec)
	for spec, bound := range bindings {
		require.Len(t, bound, 1)
		assert.Equal(t, "127.0.0.1", bound[0].HostIP,
			"%s must not be reachable beyond this machine: the hook port takes an unauthenticated payload", spec)
		assert.Empty(t, bound[0].HostPort, "the host port is ephemeral, so Docker picks it")
	}
}

func TestTheBrokerReachesTheSandboxOverLoopbackNotTheComposeAlias(t *testing.T) {
	r, _, guest := newRunner(t)

	inst, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.NoError(t, err)

	assert.Equal(t, "http://127.0.0.1:34001", inst.ControlURL)
	for _, req := range guest.requests {
		assert.Contains(t, req.url, "127.0.0.1",
			"the broker runs on the host and cannot resolve a compose alias")
	}
	assert.Contains(t, guest.requests[1].url, "34002", "the run hook goes to the hook port's binding")
}

func TestEnsureFailsWhenTheSandboxPublishedNoPorts(t *testing.T) {
	r, docker, _ := newRunner(t)
	docker.noPorts = true

	_, err := r.Ensure(context.Background(), "conv-1", "session-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "published no host port")
	assert.Len(t, docker.removed, 1, "a sandbox the broker cannot reach must not be left running")
}

func TestEnsureRefusesWithNoNetworkConfigured(t *testing.T) {
	r, docker, _ := newRunner(t)
	r.cfg.Network = ""

	_, err := r.Ensure(context.Background(), "conv-1", "session-token")
	assert.ErrorIs(t, err, ErrNoNetwork)
	assert.Zero(t, docker.created)
}

func TestEnsureRemovesTheContainerWhenTheRunHookFails(t *testing.T) {
	r, docker, guest := newRunner(t)
	guest.hookStatus = http.StatusBadRequest

	_, err := r.Ensure(context.Background(), "conv-1", "session-token")
	assert.ErrorIs(t, err, ErrRunHook)
	assert.Len(t, docker.removed, 1,
		"a sandbox with no token refuses every request, so leaving it running strands a container")
}

func TestEnsureRemovesTheContainerWhenTheGuestNeverListens(t *testing.T) {
	r, docker, guest := newRunner(t)
	guest.healthErr = errors.New("connection refused")
	r.readyTimeout = 0

	_, err := r.Ensure(context.Background(), "conv-1", "session-token")
	assert.ErrorIs(t, err, ErrNotReady)
	assert.Len(t, docker.removed, 1)
}

func TestRemoveOnANameWithNoSandboxIsNotAnError(t *testing.T) {
	r, docker, _ := newRunner(t)

	assert.NoError(t, r.Remove(context.Background(), "conv-unknown"))
	assert.Empty(t, docker.removed)
}

func TestRemoveAllDropsEverySandboxTheProjectOwns(t *testing.T) {
	r, docker, _ := newRunner(t)
	docker.list = []container.Summary{
		{ID: "a", Labels: map[string]string{labelName: "conv-1"}},
		{ID: "b", Labels: map[string]string{labelName: "conv-2"}},
	}

	require.NoError(t, r.RemoveAll(context.Background()))
	assert.ElementsMatch(t, []string{"a", "b"}, docker.removed)
}

func TestRemoveAllReportsSandboxesThatSurvived(t *testing.T) {
	r, docker, _ := newRunner(t)
	docker.list = []container.Summary{{ID: "a", Labels: map[string]string{labelName: "conv-1"}}}
	docker.removeErr = errors.New("device or resource busy")

	err := r.RemoveAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 1 survived")
}

func TestHostForIsDeterministicAndSafeAsADNSName(t *testing.T) {
	safe := regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

	for _, name := range []string{
		"conv-1",
		"thread_01J8Z9ABCDEF",
		"Conversation With Spaces",
		"../escape",
		strings.Repeat("x", 500),
		"",
	} {
		t.Run(name, func(t *testing.T) {
			host := hostFor(name)
			assert.True(t, safe.MatchString(host), "host %q is not usable as a DNS name", host)
			assert.Equal(t, host, hostFor(name), "the same name must always resolve to the same host")
		})
	}

	assert.NotEqual(t, hostFor("conv-1"), hostFor("conv-2"),
		"two attach names must not share a sandbox")
}
