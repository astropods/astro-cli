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

// useHostProxyEnvForTest makes buildDevComposeProject see env as the host's,
// on goos, with dial standing in for the proxy check.
func useHostProxyEnvForTest(t *testing.T, env map[string]string, goos string, dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	prevEnv, prevGOOS, prevDial := hostProxyGetenv, hostProxyGOOS, hostProxyDial
	hostProxyGetenv = func(k string) string { return env[k] }
	hostProxyGOOS = goos
	if dial != nil {
		hostProxyDial = dial
	}
	t.Cleanup(func() { hostProxyGetenv, hostProxyGOOS, hostProxyDial = prevEnv, prevGOOS, prevDial })
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
	useHostProxyEnvForTest(t, map[string]string{"HTTPS_PROXY": "http://" + addr}, "darwin", nil)
	var out bytes.Buffer

	project, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), t.TempDir(),
		map[string]string{"ANTHROPIC_API_KEY": "{{vault:856c3944-0ba6-5de7-a9ea-f85975c4453a}}"})

	require.NoError(t, err)
	env := project.Services["agent"].Environment
	require.NotNil(t, env["HTTPS_PROXY"])
	assert.Equal(t, "http://host.docker.internal:"+port, *env["HTTPS_PROXY"], "the loopback proxy is reached through the host gateway")
	assert.Equal(t, []string{"host-gateway"}, project.Services["agent"].ExtraHosts["host.docker.internal"])
	require.NotNil(t, env["ANTHROPIC_API_KEY"])
	assert.Equal(t, "{{vault:856c3944-0ba6-5de7-a9ea-f85975c4453a}}", *env["ANTHROPIC_API_KEY"], "values pass through untouched")
	cfg, err := hostproxy.FromEnv(hostProxyGetenv, "darwin")
	require.NoError(t, err)
	assert.Equal(t, msgHostProxyApplied(cfg), out.String())
}

func TestBuildDevComposeProject_NoHostProxyMatchesBuildProject(t *testing.T) {
	useHostProxyEnvForTest(t, map[string]string{}, "darwin", func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("must not be called")
	})
	dir := t.TempDir()
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-plain"}
	want, err := composeBuilder.BuildProject(hostProxyTestSpec(), dir, env)
	require.NoError(t, err)
	var out bytes.Buffer

	got, err := buildDevComposeProject(context.Background(), &out, hostProxyTestSpec(), dir, env)

	require.NoError(t, err)
	assert.Equal(t, want, got, "without a host proxy the project is exactly what BuildProject produces")
	assert.Empty(t, out.String())
}

func TestBuildDevComposeProject_UnreachableProxyStops(t *testing.T) {
	cause := errors.New("connection refused")
	useHostProxyEnvForTest(t, map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144"}, "darwin", func(context.Context, string, string) (net.Conn, error) {
		return nil, cause
	})

	_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, hostProxyTestSpec(), t.TempDir(), nil)

	require.Error(t, err)
	assert.Equal(t, errHostProxyUnreachable("http://127.0.0.1:10144", cause).Error(), err.Error())
}

func TestBuildDevComposeProject_LoopbackProxyOnLinuxStops(t *testing.T) {
	useHostProxyEnvForTest(t, map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144"}, "linux", nil)

	_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, hostProxyTestSpec(), t.TempDir(), nil)

	require.Error(t, err)
	assert.Equal(t, errHostProxyLoopbackOnLinux().Error(), err.Error())
}

func TestHostProxyMessages(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"unreachable", errHostProxyUnreachable("http://127.0.0.1:10144", nil).Error(),
			"The proxy at http://127.0.0.1:10144 isn't answering. Start the proxy, or set ASTRO_NO_HOST_PROXY=1 to run without it."},
		{"loopback on linux", errHostProxyLoopbackOnLinux().Error(),
			"Your proxy listens on a loopback address, which containers can't reach on Linux. Make the proxy listen on an address containers can reach, or set ASTRO_NO_HOST_PROXY=1."},
		{"CA file", errHostProxyCAFile(&hostproxy.CAFileError{Var: "SSL_CERT_FILE", Path: "/x.pem"}).Error(),
			"SSL_CERT_FILE points to a CA file that can't be used. Fix or unset SSL_CERT_FILE, or set ASTRO_NO_HOST_PROXY=1."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got)
		})
	}
}
