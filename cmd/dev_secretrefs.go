package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/secretref"
	spec "github.com/astropods/astro-spec"
)

// devSecretProvidersOverride replaces the real providers in tests.
var devSecretProvidersOverride []secretref.Provider

func devSecretProviders() ([]secretref.Provider, error) {
	if devSecretProvidersOverride != nil {
		return devSecretProvidersOverride, nil
	}
	dir, err := auth.ConfigDir(buildinfo.BinaryName)
	if err != nil {
		return nil, err
	}
	return []secretref.Provider{&secretref.Passport{BundleDir: secretref.PassportBundleDir(dir)}}, nil
}

var devProxyEnvKeys = []string{
	"HTTPS_PROXY", "HTTP_PROXY", "ALL_PROXY", "NO_PROXY",
	"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE", "NODE_USE_ENV_PROXY",
}

// warnStoredProxyEnvKeys warns about proxy and trust keys that start exports into the CLI process.
func warnStoredProxyEnvKeys(w io.Writer, env map[string]string) {
	if keys := storedProxyEnvKeys(env); len(keys) > 0 {
		fmt.Fprint(w, msgDevEnvProxyKeysStored(keys)) //nolint:errcheck,gosec
	}
}

func storedProxyEnvKeys(env map[string]string) []string {
	var found []string
	for k := range env {
		if slices.Contains(devProxyEnvKeys, strings.ToUpper(k)) {
			found = append(found, k)
		}
	}
	slices.Sort(found)
	return found
}

// resolveDevSecretRefs replaces secret references in envVars and returns the
// runtime the containers need. It never touches the CLI's own process env.
func resolveDevSecretRefs(ctx context.Context, w io.Writer, envVars map[string]string) (map[string]string, *secretref.ContainerRuntime, error) {
	providers, err := devSecretProviders()
	if err != nil {
		return nil, nil, err
	}
	plan, err := secretref.Resolve(ctx, providers, envVars)
	var checkErr *secretref.CheckError
	if errors.As(err, &checkErr) {
		return nil, nil, errSecretRefCheck(checkErr)
	}
	if err != nil {
		return nil, nil, err
	}
	for _, u := range plan.Uses {
		fmt.Fprint(w, msgSecretRefsResolved(u)) //nolint:errcheck,gosec
	}
	return plan.Env, plan.Runtime, nil
}

// buildDevComposeProject resolves secret references, builds the compose
// project, and applies the providers' container runtime to it.
func buildDevComposeProject(ctx context.Context, w io.Writer, s *spec.AstroSpec, workingDir string, envVars map[string]string) (*types.Project, error) {
	envVars, rt, err := resolveDevSecretRefs(ctx, w, envVars)
	if err != nil {
		return nil, err
	}
	project, err := composeBuilder.BuildProject(s, workingDir, envVars, composeBuilder.WithWarnings(w))
	if err != nil {
		return nil, fmt.Errorf("failed to build compose project: %w", err)
	}
	if keys := composeBuilder.ApplyRuntime(project, rt); len(keys) > 0 {
		fmt.Fprint(w, msgSecretRefRuntimeOverridden(keys)) //nolint:errcheck,gosec
	}
	return project, nil
}
