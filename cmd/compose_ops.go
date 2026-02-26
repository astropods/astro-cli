package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
)

// newComposeService creates a compose API service backed by the local Docker daemon.
// When verbose is true, progress events (pulling, creating, starting) are logged to stdout.
func newComposeService(verbose bool) (api.Compose, error) {
	cliOpts := []command.CLIOption{}
	if !verbose {
		cliOpts = append(cliOpts, command.WithCombinedStreams(io.Discard))
	}
	dockerCli, err := command.NewDockerCli(cliOpts...)
	if err != nil {
		return nil, fmt.Errorf("init docker CLI: %w", err)
	}
	if err := dockerCli.Initialize(cliflags.NewClientOptions()); err != nil {
		return nil, fmt.Errorf("init docker CLI: %w", err)
	}
	opts := []compose.Option{}
	if verbose {
		opts = append(opts, compose.WithEventProcessor(&loggingEventProcessor{}))
	}
	svc, err := compose.NewComposeService(dockerCli, opts...)
	if err != nil {
		return nil, fmt.Errorf("init compose service: %w", err)
	}
	return svc, nil
}

// loggingEventProcessor implements api.EventProcessor, printing compose SDK events to stdout.
type loggingEventProcessor struct{}

func (p *loggingEventProcessor) Start(_ context.Context, _ string) {}

func (p *loggingEventProcessor) On(events ...api.Resource) {
	for _, e := range events {
		switch e.Status {
		case api.Done:
			_, _ = fmt.Fprintf(os.Stdout, "   %s %s\n", composeEventIcon(e.Text), e.ID)
		case api.Warning:
			_, _ = fmt.Fprintf(os.Stderr, "   ⚠️  %s\n", e.ID)
		case api.Error:
			_, _ = fmt.Fprintf(os.Stderr, "   ❌ %s %s\n", e.ID, e.Text)
		}
	}
}

func composeEventIcon(text string) string {
	switch text {
	case api.StatusPulled:
		return "⬇️ "
	case api.StatusBuilt:
		return "🔨"
	case api.StatusCreated:
		return "📦"
	case api.StatusStarted, api.StatusRunning:
		return "✅"
	case api.StatusHealthy:
		return "💚"
	case api.StatusStopped, api.StatusKilled, api.StatusExited:
		return "⏹️ "
	case api.StatusRemoved:
		return "🗑️ "
	default:
		return "✅"
	}
}

func (p *loggingEventProcessor) Done(operation string, failed bool) {
	if failed {
		_, _ = fmt.Fprintf(os.Stderr, "   ❌ %s failed\n", operation)
	}
}

// projectForUp returns a shallow copy of p with profiled services excluded,
// matching the behaviour of `docker compose up` (without --profile ingestion).
func projectForUp(p *types.Project) *types.Project {
	up := *p
	up.Services = make(types.Services)
	for name, svc := range p.Services {
		if len(svc.Profiles) == 0 {
			up.Services[name] = svc
		}
	}
	return &up
}

// allServiceNames returns the names of all services in the project.
func allServiceNames(p *types.Project) []string {
	names := make([]string, 0, len(p.Services))
	for name := range p.Services {
		names = append(names, name)
	}
	return names
}

// stdoutLogConsumer implements api.LogConsumer, forwarding logs to out/err writers.
type stdoutLogConsumer struct {
	out io.Writer
	err io.Writer
}

func (c *stdoutLogConsumer) Log(container, msg string)    { _, _ = fmt.Fprintf(c.out, "%s  | %s\n", container, msg) }
func (c *stdoutLogConsumer) Err(container, msg string)    { _, _ = fmt.Fprintf(c.err, "%s  | %s\n", container, msg) }
func (c *stdoutLogConsumer) Status(container, msg string) { _, _ = fmt.Fprintf(c.out, "%s %s\n", container, msg) }
