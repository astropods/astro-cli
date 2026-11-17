package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/hostproxy"
	spec "github.com/astropods/astro-spec"
)

// Replaced in tests.
var (
	hostEnvSnapshot = snapshotEnv
	hostProxyGOOS   = runtime.GOOS
	hostProxyDial   = (&net.Dialer{Timeout: 2 * time.Second}).DialContext
)

// snapshotEnv captures the process env as it is now. Start takes it before
// assembleDevEnv exports project variables, so only the developer's own
// settings count as the host's.
func snapshotEnv() func(string) string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return func(k string) string { return env[k] }
}

// buildDevComposeProject builds the compose project and routes the agent and
// ingestion containers through the proxy in hostEnv, when it names one.
func buildDevComposeProject(ctx context.Context, w io.Writer, s *spec.AstroSpec, workingDir string, envVars map[string]string, hostEnv func(string) string) (*types.Project, error) {
	project, err := composeBuilder.BuildProject(s, workingDir, envVars, composeBuilder.WithWarnings(w))
	if err != nil {
		return nil, fmt.Errorf("failed to build compose project: %w", err)
	}

	cfg, err := hostproxy.FromEnv(hostEnv, hostProxyGOOS)
	var invalid *hostproxy.InvalidProxyError
	switch {
	case errors.Is(err, hostproxy.ErrLoopbackOnLinux):
		fmt.Fprint(w, msgHostProxyLoopbackSkipped()) //nolint:errcheck,gosec
		return project, nil
	case errors.As(err, &invalid):
		return nil, errHostProxyInvalid(invalid.Var)
	case err != nil:
		return nil, err
	case cfg == nil:
		return project, nil
	}

	if p, err := cfg.Check(ctx, hostProxyDial); err != nil {
		return nil, errHostProxyUnreachable(p, err)
	}
	dir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return nil, err
	}
	rt, err := cfg.Runtime(filepath.Join(dir, "proxy"))
	var caErr *hostproxy.CAFileError
	if errors.As(err, &caErr) {
		return nil, errHostProxyCAFile(caErr)
	}
	if err != nil {
		return nil, err
	}

	fmt.Fprint(w, msgHostProxyApplied(cfg)) //nolint:errcheck,gosec
	if keys := composeBuilder.ApplyRuntime(project, rt); len(keys) > 0 {
		fmt.Fprint(w, msgHostProxyOverridden(keys)) //nolint:errcheck,gosec
	}
	return project, nil
}
