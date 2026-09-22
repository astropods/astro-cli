// Package sandbox runs agent sandboxes locally for `ast dev`.
//
// One container per attach name, from the same image the deployed sandbox uses.
// The container is driven exactly as Lambda drives a MicroVM: the run hook
// installs the session token before any request reaches the data plane.
//
// See docs/01-spec/ast-dev-sandbox-spec.md in astropods/astro.
package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// DataPlanePort is the port sandboxd serves the agent-facing API on. It
	// matches the deployed sandbox's EnvdPort.
	DataPlanePort = 49983

	// HookPort is the port the platform calls lifecycle hooks on. The deployed
	// sandbox receives them here, and no minted credential reaches it.
	HookPort = 9000

	// RuntimePrefix is where the platform calls the hooks. It appears in no AWS
	// API, and the local runner uses it so the guest serves one path.
	RuntimePrefix = "/aws/lambda-microvms/runtime/v1"

	// RuntimeTag names the runtime that produced any stored state. It differs
	// from the deployed tag, so a workspace cannot move between them.
	RuntimeTag = "docker-1"
)

// Labels identify the containers this package owns. The labels are the source
// of truth for which sandbox belongs to which attach name: the worker holds no
// map, so a restarted worker still finds a running sandbox.
const (
	labelManaged = "ai.astropods.sandbox"
	labelProject = "ai.astropods.sandbox.project"
	labelName    = "ai.astropods.sandbox.name"
)

var (
	ErrNotFound  = errors.New("sandbox not found")
	ErrNotReady  = errors.New("sandbox did not become ready")
	ErrRunHook   = errors.New("sandbox refused the run hook")
	ErrNoNetwork = errors.New("sandbox network is not configured")
)

// Docker is the subset of the Docker API this package uses.
type Docker interface {
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig,
		networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, name string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, id string, options container.StartOptions) error
	ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerRemove(ctx context.Context, id string, options container.RemoveOptions) error
}

// Config configures the runner.
type Config struct {
	// Image is the sandbox image, e.g. astropods/sandbox:<tag>.
	Image string
	// Network is the compose network the agent runs on. The sandbox joins it so
	// the agent reaches the sandbox by name and no host port is allocated.
	Network string
	// Project scopes the labels, so one `ast dev` never removes another's
	// sandboxes.
	Project string
}

type Runner struct {
	docker Docker
	cfg    Config
	http   httpDoer

	readyTimeout  time.Duration
	readyInterval time.Duration
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewRunner(docker Docker, cfg Config) *Runner {
	return &Runner{
		docker:        docker,
		cfg:           cfg,
		http:          &http.Client{Timeout: 5 * time.Second},
		readyTimeout:  60 * time.Second,
		readyInterval: 250 * time.Millisecond,
	}
}

// Instance describes a running sandbox.
//
// The agent and the broker reach the sandbox by different routes. The agent
// runs in a container on the compose network and uses Endpoint, which costs no
// host port and no hop. The broker runs on the host and cannot resolve a
// compose alias, so it uses ControlURL, a loopback port Docker publishes.
type Instance struct {
	Name        string
	ContainerID string
	// Host is the sandbox's alias on the compose network.
	Host string
	// Endpoint is what the agent receives in its handle.
	Endpoint string
	// ControlURL reaches the data plane from the host.
	ControlURL string
	// hookURL reaches the hook server from the host.
	hookURL string
	// Started reports whether this call created the container, so the caller
	// knows the run hook has just installed a fresh token.
	Started bool
}

// hostFor derives a DNS-safe network alias from an attach name. An attach name
// is caller-supplied, usually a conversation ID, so it can hold characters
// Docker refuses. The digest keeps distinct names distinct.
func hostFor(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "sbx-" + hex.EncodeToString(sum[:])[:12]
}

// Ensure returns a ready sandbox for name, creating one if none is running.
// Creating one installs token through the run hook before returning.
func (r *Runner) Ensure(ctx context.Context, name, token string) (Instance, error) {
	if r.cfg.Network == "" {
		return Instance{}, ErrNoNetwork
	}

	host := hostFor(name)

	existing, err := r.find(ctx, name)
	if err == nil {
		if existing.State == "running" {
			inst := Instance{
				Name:        name,
				ContainerID: existing.ID,
				Host:        host,
				Endpoint:    endpointFor(host),
			}
			if err := r.published(ctx, &inst); err != nil {
				return Instance{}, err
			}
			return inst, nil
		}
		// A stopped sandbox cannot serve the data plane, and its token was
		// installed in memory that is gone. Replace it.
		if err := r.removeID(ctx, existing.ID); err != nil {
			return Instance{}, err
		}
	} else if !errors.Is(err, ErrNotFound) {
		return Instance{}, err
	}

	id, err := r.create(ctx, name, host)
	if err != nil {
		return Instance{}, err
	}

	if err := r.docker.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		_ = r.removeID(ctx, id)
		return Instance{}, fmt.Errorf("start the sandbox: %w", err)
	}

	inst := Instance{Name: name, ContainerID: id, Host: host, Endpoint: endpointFor(host), Started: true}

	if err := r.published(ctx, &inst); err != nil {
		_ = r.removeID(ctx, id)
		return Instance{}, err
	}

	if err := r.waitListening(ctx, inst); err != nil {
		_ = r.removeID(ctx, id)
		return Instance{}, err
	}
	if err := r.runHook(ctx, inst, name, token); err != nil {
		_ = r.removeID(ctx, id)
		return Instance{}, err
	}
	return inst, nil
}

func endpointFor(host string) string {
	return fmt.Sprintf("http://%s:%d", host, DataPlanePort)
}

var (
	dataPlanePortSpec = nat.Port(fmt.Sprintf("%d/tcp", DataPlanePort))
	hookPortSpec      = nat.Port(fmt.Sprintf("%d/tcp", HookPort))
)

// published reads the loopback ports Docker assigned, which exist only after
// the container starts.
func (r *Runner) published(ctx context.Context, inst *Instance) error {
	info, err := r.docker.ContainerInspect(ctx, inst.ContainerID)
	if err != nil {
		return fmt.Errorf("inspect the sandbox: %w", err)
	}
	if info.NetworkSettings == nil {
		return fmt.Errorf("the sandbox published no ports")
	}

	data, err := loopbackPort(info.NetworkSettings.Ports, dataPlanePortSpec)
	if err != nil {
		return err
	}
	hook, err := loopbackPort(info.NetworkSettings.Ports, hookPortSpec)
	if err != nil {
		return err
	}
	inst.ControlURL = "http://127.0.0.1:" + data
	inst.hookURL = "http://127.0.0.1:" + hook
	return nil
}

func loopbackPort(ports nat.PortMap, spec nat.Port) (string, error) {
	bindings := ports[spec]
	if len(bindings) == 0 || bindings[0].HostPort == "" {
		return "", fmt.Errorf("the sandbox published no host port for %s", spec)
	}
	return bindings[0].HostPort, nil
}

func (r *Runner) create(ctx context.Context, name, host string) (string, error) {
	res, err := r.docker.ContainerCreate(ctx,
		&container.Config{
			Image: r.cfg.Image,
			Labels: map[string]string{
				labelManaged: "true",
				labelProject: r.cfg.Project,
				labelName:    name,
			},
		},
		&container.HostConfig{
			// Loopback only. The hook port accepts an unauthenticated run
			// payload, and in the deployed sandbox no minted credential reaches
			// it, so it must not be published beyond this machine.
			PortBindings: nat.PortMap{
				dataPlanePortSpec: []nat.PortBinding{{HostIP: "127.0.0.1"}},
				hookPortSpec:      []nat.PortBinding{{HostIP: "127.0.0.1"}},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				r.cfg.Network: {Aliases: []string{host}},
			},
		},
		nil,
		host,
	)
	if err != nil {
		return "", fmt.Errorf("create the sandbox: %w", err)
	}
	return res.ID, nil
}

func (r *Runner) find(ctx context.Context, name string) (container.Summary, error) {
	list, err := r.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(r.labelFilters(), filters.Arg("label", labelName+"="+name)),
	})
	if err != nil {
		return container.Summary{}, fmt.Errorf("list sandboxes: %w", err)
	}
	if len(list) == 0 {
		return container.Summary{}, ErrNotFound
	}
	return list[0], nil
}

func (r *Runner) labelFilters() filters.KeyValuePair {
	return filters.Arg("label", labelProject+"="+r.cfg.Project)
}

// Remove drops the sandbox for name. A name with no sandbox is not an error,
// because teardown runs over every name the broker has seen.
func (r *Runner) Remove(ctx context.Context, name string) error {
	found, err := r.find(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.removeID(ctx, found.ID)
}

// RemoveAll drops every sandbox this project owns. `ast dev` down calls it.
func (r *Runner) RemoveAll(ctx context.Context) error {
	list, err := r.docker.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(r.labelFilters()),
	})
	if err != nil {
		return fmt.Errorf("list sandboxes: %w", err)
	}
	var failed int
	for _, c := range list {
		if err := r.removeID(ctx, c.ID); err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("remove sandboxes: %d of %d survived", failed, len(list))
	}
	return nil
}

func (r *Runner) removeID(ctx context.Context, id string) error {
	err := r.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: true, RemoveVolumes: true})
	if err != nil {
		return fmt.Errorf("remove the sandbox: %w", err)
	}
	return nil
}

// waitListening polls the guest's health endpoint until sandboxd binds. The
// container reports running before its process listens.
func (r *Runner) waitListening(ctx context.Context, inst Instance) error {
	deadline := time.Now().Add(r.readyTimeout)
	var last error
	for {
		ok, err := r.probe(ctx, inst)
		if ok {
			return nil
		}
		last = err
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %w", ErrNotReady, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.readyInterval):
		}
	}
}

func (r *Runner) probe(ctx context.Context, inst Instance) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, inst.ControlURL+"/healthz", nil)
	if err != nil {
		return false, err
	}
	res, err := r.http.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close() //nolint:errcheck // the status is what matters
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("health returned %d", res.StatusCode)
	}
	return true, nil
}

// runHook installs the session token the way the platform does, so the guest
// serves one path and holds no local-only branch.
func (r *Runner) runHook(ctx context.Context, inst Instance, name, token string) error {
	payload, err := json.Marshal(map[string]string{"sandbox_id": name, "envd_token": token})
	if err != nil {
		return fmt.Errorf("marshal the run payload: %w", err)
	}
	body, err := json.Marshal(map[string]string{
		"microvmId":      inst.ContainerID,
		"runHookPayload": string(payload),
	})
	if err != nil {
		return fmt.Errorf("marshal the run envelope: %w", err)
	}

	url := inst.hookURL + RuntimePrefix + "/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build the run request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := r.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRunHook, err)
	}
	defer res.Body.Close() //nolint:errcheck // the status is what matters
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: the sandbox answered %d", ErrRunHook, res.StatusCode)
	}
	return nil
}
