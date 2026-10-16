package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/sirupsen/logrus"
)

func newComposeService(verbose bool) (api.Compose, error) {
	cliOpts := []command.CLIOption{}
	if !verbose {
		cliOpts = append(cliOpts, command.WithCombinedStreams(io.Discard))
	}
	return buildComposeService(cliOpts, verbose)
}

// newJobComposeService returns a compose service whose one-off containers write
// their stdout and stderr to out and errOut.
func newJobComposeService(out, errOut io.Writer, verbose bool) (api.Compose, error) {
	return buildComposeService([]command.CLIOption{
		command.WithOutputStream(out),
		command.WithErrorStream(errOut),
	}, verbose)
}

func buildComposeService(cliOpts []command.CLIOption, verbose bool) (api.Compose, error) {
	dockerCli, err := command.NewDockerCli(cliOpts...)
	if err != nil {
		return nil, fmt.Errorf("init docker CLI: %w", err)
	}
	clientOpts := cliflags.NewClientOptions()
	clientOpts.LogLevel = logrus.GetLevel().String()
	if err := dockerCli.Initialize(clientOpts); err != nil {
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
// matching the behavior of `docker compose up` (without --profile ingestion).
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

// projectForRun returns a copy of p with the services `up` starts plus the target.
// Compose starts everything left in the project, and RunOptions.NoDeps does not gate it.
func projectForRun(p *types.Project, service string) *types.Project {
	run := *p
	run.Services = make(types.Services)
	for name, svc := range p.Services {
		if len(svc.Profiles) == 0 || name == service {
			run.Services[name] = svc
		}
	}
	return &run
}

// groupStartupServices splits services into the ones `up` starts and the
// profiled ingestions it does not, tagging each ingestion with when it runs.
func groupStartupServices(p *types.Project, s *spec.AstroSpec) (services, ingestion []string) {
	services = make([]string, 0, len(p.Services))
	for name, svc := range p.Services {
		if len(svc.Profiles) == 0 {
			services = append(services, name)
			continue
		}
		when := "on demand"
		if ing, ok := s.Ingestion[strings.TrimPrefix(name, "ingestion-")]; ok && ing.Trigger.Type == "startup" {
			when = "once at start"
		}
		ingestion = append(ingestion, fmt.Sprintf("%s (%s)", name, when))
	}
	sort.Strings(services)
	sort.Strings(ingestion)
	return services, ingestion
}

// buildWithIngestionProfile builds p with the "ingestion" profile active, so
// the SDK resolves the profiled ingestion services by name.
func buildWithIngestionProfile(svc api.Compose, p *types.Project, opts api.BuildOptions) error {
	build := *p
	build.Profiles = []string{"ingestion"}
	return svc.Build(context.Background(), &build, opts)
}

// startupIngestionNames returns the sorted names of ingestions with a startup trigger.
func startupIngestionNames(s *spec.AstroSpec) []string {
	var names []string
	for name, ingestion := range s.Ingestion {
		if ingestion.Trigger.Type == "startup" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// foregroundLogServices returns the services `project start` tails by default:
// the agent and every ingestion, so a job run while the agent is up shows in
// the same output.
func foregroundLogServices(s *spec.AstroSpec) []string {
	services := []string{"agent"}
	ingestions := make([]string, 0, len(s.Ingestion))
	for name := range s.Ingestion {
		ingestions = append(ingestions, "ingestion-"+name)
	}
	sort.Strings(ingestions)
	return append(services, ingestions...)
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

func (c *stdoutLogConsumer) Log(container, msg string) {
	_, _ = fmt.Fprintf(c.out, "%s  | %s\n", container, msg)
}
func (c *stdoutLogConsumer) Err(container, msg string) {
	_, _ = fmt.Fprintf(c.err, "%s  | %s\n", container, msg)
}
func (c *stdoutLogConsumer) Status(container, msg string) {
	_, _ = fmt.Fprintf(c.out, "%s %s\n", container, msg)
}
