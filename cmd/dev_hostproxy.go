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
	hostProxyGetenv = os.Getenv
	hostProxyGOOS   = runtime.GOOS
	hostProxyDial   = (&net.Dialer{Timeout: 2 * time.Second}).DialContext
)

// buildDevComposeProject builds the compose project and routes the agent and
// ingestion containers through the host's proxy, when the host has one.
func buildDevComposeProject(ctx context.Context, w io.Writer, s *spec.AstroSpec, workingDir string, envVars map[string]string) (*types.Project, error) {
	project, err := composeBuilder.BuildProject(s, workingDir, envVars, composeBuilder.WithWarnings(w))
	if err != nil {
		return nil, fmt.Errorf("failed to build compose project: %w", err)
	}

	cfg, err := hostproxy.FromEnv(hostProxyGetenv, hostProxyGOOS)
	switch {
	case errors.Is(err, hostproxy.ErrLoopbackOnLinux):
		return nil, errHostProxyLoopbackOnLinux()
	case err != nil:
		return nil, errHostProxyInvalid(err)
	case cfg == nil:
		return project, nil
	}

	if err := cfg.Check(ctx, hostProxyDial); err != nil {
		return nil, errHostProxyUnreachable(cfg.HostURL, err)
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
