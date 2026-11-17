package cmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	composeBuilder "github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/hostproxy"
)

func hostProxyTestSpec() *spec.AstroSpec {
	return &spec.AstroSpec{
		Name:   "proxy-agent",
		Agent:  spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{"anthropic": {Provider: "anthropic"}},
	}
}

func hostEnvOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// useHostProxyTestHooks pins the OS and, when dial is set, the proxy check.
func useHostProxyTestHooks(t *testing.T, goos string, dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	prevGOOS, prevDial := hostProxyGOOS, hostProxyDial
	hostProxyGOOS = goos
	if dial != nil {
		hostProxyDial = dial
	}
	t.Cleanup(func() { hostProxyGOOS, hostProxyDial = prevGOOS, prevDial })
}

func listeningProxy(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func TestBuildDevComposeProject_HostProxyRoutesTheAgent(t *testing.T) {
	addr := listeningProxy(t)
	_, port, _ := net.SplitHostPort(addr)
	useHostProxyTestHooks(t, "darwin", nil)
	hostEnv := hostEnvOf(map[string]string{"HTTPS_PROXY": "http://" + addr})
	var out bytes.Buffer

	project, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), t.TempDir(),
		map[string]string{"ANTHROPIC_API_KEY": "{{vault:856c3944-0ba6-5de7-a9ea-f85975c4453a}}"}, hostEnv)

	require.NoError(t, err)
	env := project.Services["agent"].Environment
	require.NotNil(t, env["HTTPS_PROXY"])
	assert.Equal(t, "http://host.docker.internal:"+port, *env["HTTPS_PROXY"], "the loopback proxy is reached through the host gateway")
	assert.Equal(t, []string{"host-gateway"}, project.Services["agent"].ExtraHosts["host.docker.internal"])
	require.NotNil(t, env["ANTHROPIC_API_KEY"])
	assert.Equal(t, "{{vault:856c3944-0ba6-5de7-a9ea-f85975c4453a}}", *env["ANTHROPIC_API_KEY"], "values pass through untouched")
	cfg, err := hostproxy.FromEnv(hostEnv, "darwin")
	require.NoError(t, err)
	assert.Equal(t, msgHostProxyApplied(cfg), out.String())
}

func TestBuildDevComposeProject_NoHostProxyMatchesBuildProject(t *testing.T) {
	useHostProxyTestHooks(t, "darwin", func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("must not be called")
	})
	dir := t.TempDir()
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-plain"}
	want, err := composeBuilder.BuildProject(hostProxyTestSpec(), dir, env)
	require.NoError(t, err)
	var out bytes.Buffer

	got, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), dir, env, hostEnvOf(nil))

	require.NoError(t, err)
	assert.Equal(t, want, got, "without a host proxy the project is exactly what BuildProject produces")
	assert.Empty(t, out.String())
}

func TestHostEnvSnapshot_IgnoresProjectVarsExportedAfterIt(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("SSL_CERT_FILE", "")
	snapshot := hostEnvSnapshot()
	t.Setenv("HTTPS_PROXY", "http://proxy.from-project-vars:3128")
	t.Setenv("SSL_CERT_FILE", "/project/ca.pem")
	useHostProxyTestHooks(t, "darwin", func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("must not be called")
	})
	dir := t.TempDir()
	want, err := composeBuilder.BuildProject(hostProxyTestSpec(), dir, nil)
	require.NoError(t, err)
	var out bytes.Buffer

	got, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), dir, nil, snapshot)

	require.NoError(t, err)
	assert.Equal(t, want, got, "a proxy that only exists in exported project vars doesn't route the containers")
	assert.Empty(t, out.String())
}

func TestBuildDevComposeProject_UnreachableProxyStops(t *testing.T) {
	cause := errors.New("connection refused")
	useHostProxyTestHooks(t, "darwin", func(context.Context, string, string) (net.Conn, error) { return nil, cause })
	hostEnv := hostEnvOf(map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144"})

	_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, hostProxyTestSpec(), t.TempDir(), nil, hostEnv)

	require.Error(t, err)
	cfg, ferr := hostproxy.FromEnv(hostEnv, "darwin")
	require.NoError(t, ferr)
	assert.Equal(t, errHostProxyUnreachable(cfg.HTTPS, cause).Error(), err.Error())
}

func TestBuildDevComposeProject_LoopbackProxyOnLinuxIsSkipped(t *testing.T) {
	useHostProxyTestHooks(t, "linux", func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("must not be called")
	})
	dir := t.TempDir()
	want, err := composeBuilder.BuildProject(hostProxyTestSpec(), dir, nil)
	require.NoError(t, err)
	var out bytes.Buffer

	got, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), dir, nil,
		hostEnvOf(map[string]string{"HTTPS_PROXY": "http://127.0.0.1:3128"}))

	require.NoError(t, err, "a Linux user with a local proxy still gets a dev environment")
	assert.Equal(t, want, got, "the project runs without the proxy")
	assert.Equal(t, msgHostProxyLoopbackSkipped(), out.String())
}

func TestBuildDevComposeProject_CredentialsNeverPrinted(t *testing.T) {
	addr := listeningProxy(t)
	hostEnv := hostEnvOf(map[string]string{"HTTPS_PROXY": "http://alice:s3cret@" + addr})

	t.Run("applied", func(t *testing.T) {
		useHostProxyTestHooks(t, "darwin", nil)
		var out bytes.Buffer

		project, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), t.TempDir(), nil, hostEnv)

		require.NoError(t, err)
		assert.NotContains(t, out.String(), "s3cret")
		assert.Contains(t, *project.Services["agent"].Environment["HTTPS_PROXY"], "alice:s3cret@", "the container still gets the credentials")
	})
	t.Run("unreachable", func(t *testing.T) {
		useHostProxyTestHooks(t, "darwin", func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		})

		_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, hostProxyTestSpec(), t.TempDir(), nil, hostEnv)

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "s3cret")
	})
	t.Run("invalid", func(t *testing.T) {
		useHostProxyTestHooks(t, "darwin", nil)

		_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, hostProxyTestSpec(), t.TempDir(), nil,
			hostEnvOf(map[string]string{"HTTPS_PROXY": "http://alice:s3cret@:3128"}))

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "s3cret")
		assert.Equal(t, errHostProxyInvalid("HTTPS_PROXY").Error(), err.Error())
	})
}

func TestBuildDevComposeProject_UnusableCAFileStops(t *testing.T) {
	addr := listeningProxy(t)
	useHostProxyTestHooks(t, "darwin", nil)

	_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, hostProxyTestSpec(), t.TempDir(), nil,
		hostEnvOf(map[string]string{"HTTPS_PROXY": "http://" + addr, "SSL_CERT_FILE": "/does/not/exist.pem"}))

	var ce *cliError
	require.ErrorAs(t, err, &ce, "a bad CA file is reported as a CLI error, not a raw one")
	assert.Equal(t, "SSL_CERT_FILE points to a CA file that can't be used", ce.headline)
}

func TestHostProxyMessages(t *testing.T) {
	cfg, err := hostproxy.FromEnv(hostEnvOf(map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144"}), "darwin")
	require.NoError(t, err)
	https := cfg.HTTPS
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"unreachable", errHostProxyUnreachable(https, nil).Error(),
			"The proxy in HTTPS_PROXY (http://127.0.0.1:10144) isn't answering. Start the proxy, or set ASTRO_NO_HOST_PROXY=1 to run without it."},
		{"invalid names the variable", errHostProxyInvalid("http_proxy").Error(),
			"http_proxy isn't a valid proxy URL. Fix http_proxy, or set ASTRO_NO_HOST_PROXY=1."},
		{"CA file", errHostProxyCAFile(&hostproxy.CAFileError{Var: "SSL_CERT_FILE", Path: "/x.pem"}).Error(),
			"SSL_CERT_FILE points to a CA file that can't be used. Fix or unset SSL_CERT_FILE, or set ASTRO_NO_HOST_PROXY=1."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got)
		})
	}
}
