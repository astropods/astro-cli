package hostproxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/envvar"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestFromEnv(t *testing.T) {
	cases := []struct {
		name         string
		env          map[string]string
		goos         string
		wantNil      bool
		wantErr      error
		wantHost     string
		wantInside   string
		wantLoopback bool
		wantNoProxy  []string
	}{
		{name: "no proxy set", env: map[string]string{}, wantNil: true},
		{name: "opted out", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", envvar.NoHostProxy: "1"}, wantNil: true},
		{name: "opt-out set to 0 keeps the proxy", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", envvar.NoHostProxy: "0"},
			wantHost: "http://127.0.0.1:10144", wantInside: "http://host.docker.internal:10144", wantLoopback: true},
		{name: "127.0.0.1 is rewritten for containers", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", "NO_PROXY": "localhost, .internal ,"},
			wantHost: "http://127.0.0.1:10144", wantInside: "http://host.docker.internal:10144", wantLoopback: true, wantNoProxy: []string{"localhost", ".internal"}},
		{name: "localhost is rewritten", env: map[string]string{"https_proxy": "http://localhost:3128"},
			wantHost: "http://localhost:3128", wantInside: "http://host.docker.internal:3128", wantLoopback: true},
		{name: "IPv6 loopback is rewritten", env: map[string]string{"HTTPS_PROXY": "http://[::1]:3128"},
			wantHost: "http://[::1]:3128", wantInside: "http://host.docker.internal:3128", wantLoopback: true},
		{name: "remote proxy is used as is", env: map[string]string{"HTTPS_PROXY": "http://proxy.corp.example:3128"},
			wantHost: "http://proxy.corp.example:3128", wantInside: "http://proxy.corp.example:3128"},
		{name: "HTTP_PROXY alone counts", env: map[string]string{"HTTP_PROXY": "http://proxy.corp.example:3128"},
			wantHost: "http://proxy.corp.example:3128", wantInside: "http://proxy.corp.example:3128"},
		{name: "a URL without a scheme gets http", env: map[string]string{"HTTPS_PROXY": "proxy.corp.example:3128"},
			wantHost: "http://proxy.corp.example:3128", wantInside: "http://proxy.corp.example:3128"},
		{name: "loopback on linux is refused", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144"}, goos: "linux", wantErr: ErrLoopbackOnLinux},
		{name: "remote proxy on linux is fine", env: map[string]string{"HTTPS_PROXY": "http://proxy.corp.example:3128"}, goos: "linux",
			wantHost: "http://proxy.corp.example:3128", wantInside: "http://proxy.corp.example:3128"},
		{name: "a URL with no host is invalid", env: map[string]string{"HTTPS_PROXY": "http://:3128"}, wantErr: ErrInvalidProxyURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goos := tc.goos
			if goos == "" {
				goos = "darwin"
			}
			cfg, err := FromEnv(envOf(tc.env), goos)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, cfg)
				return
			}
			require.NotNil(t, cfg)
			assert.Equal(t, tc.wantHost, cfg.HostURL)
			assert.Equal(t, tc.wantInside, cfg.ContainerURL)
			assert.Equal(t, tc.wantLoopback, cfg.Loopback)
			assert.Equal(t, tc.wantNoProxy, cfg.NoProxy)
		})
	}
}

func TestCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	dial := (&net.Dialer{Timeout: time.Second}).DialContext

	up := &Config{HostURL: "http://" + ln.Addr().String()}
	assert.NoError(t, up.Check(context.Background(), dial), "a listening proxy passes")

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := closed.Addr().String()
	require.NoError(t, closed.Close())
	down := &Config{HostURL: "http://" + addr}
	assert.Error(t, down.Check(context.Background(), dial), "a port nothing listens on fails")
}

func writeTestCA(t *testing.T, dir, name string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	path := filepath.Join(dir, name+".pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	return path
}

func TestRuntime_RoutesAndTrustsTheHostCAs(t *testing.T) {
	ca := writeTestCA(t, t.TempDir(), "passport-proxy")
	bundleDir := t.TempDir()
	cfg := &Config{
		HostURL: "http://127.0.0.1:10144", ContainerURL: "http://host.docker.internal:10144", Loopback: true,
		NoProxy: []string{".internal"},
		CAFiles: map[string]string{"SSL_CERT_FILE": ca, "NODE_EXTRA_CA_CERTS": ca},
	}

	rt, err := cfg.Runtime(bundleDir)

	require.NoError(t, err)
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		assert.Equal(t, "http://host.docker.internal:10144", rt.Env[k], "%s uses the container-side URL", k)
	}
	assert.Equal(t, "1", rt.Env["NODE_USE_ENV_PROXY"], "Node 24 bypasses HTTPS_PROXY without it")
	assert.Equal(t, []string{"localhost", "127.0.0.1", ".internal"}, rt.NoProxy)
	assert.Equal(t, []string{"host.docker.internal:host-gateway"}, rt.ExtraHosts, "a loopback proxy needs the host mapping")
	for _, k := range []string{"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE"} {
		assert.Equal(t, caBundleMountPath, rt.Env[k], "%s points at the combined bundle", k)
	}
	require.Len(t, rt.Mounts, 1)
	assert.Equal(t, filepath.Join(bundleDir, "ca-bundle.pem"), rt.Mounts[0].Source)

	data, err := os.ReadFile(rt.Mounts[0].Source)
	require.NoError(t, err)
	var names []string
	for block, rest := pem.Decode(data); block != nil; block, rest = pem.Decode(rest) {
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		names = append(names, cert.Subject.CommonName)
	}
	assert.Greater(t, len(names), 100, "the bundle carries the public roots")
	assert.Contains(t, names, "ISRG Root X1")
	count := 0
	for _, n := range names {
		if n == "passport-proxy" {
			count++
		}
	}
	assert.Equal(t, 1, count, "a CA file named by two variables is added once")
}

func TestRuntime_NoCAFilesMeansNoBundle(t *testing.T) {
	cfg := &Config{HostURL: "http://proxy.corp.example:3128", ContainerURL: "http://proxy.corp.example:3128", CAFiles: map[string]string{}}

	rt, err := cfg.Runtime(t.TempDir())

	require.NoError(t, err)
	assert.Empty(t, rt.Mounts)
	assert.Empty(t, rt.ExtraHosts, "a remote proxy needs no host mapping")
	assert.NotContains(t, rt.Env, "SSL_CERT_FILE", "the container keeps its own trust store")
}

func TestRuntime_MissingCAFile(t *testing.T) {
	cfg := &Config{HostURL: "http://127.0.0.1:1", ContainerURL: "http://host.docker.internal:1",
		CAFiles: map[string]string{"SSL_CERT_FILE": filepath.Join(t.TempDir(), "missing.pem")}}

	_, err := cfg.Runtime(t.TempDir())

	var caErr *CAFileError
	require.ErrorAs(t, err, &caErr)
	assert.Equal(t, "SSL_CERT_FILE", caErr.Var)
}

func TestWriteCABundle_RejectsNonCertificateInput(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(bad, []byte("not a cert"), 0o600))

	_, err := WriteCABundle(filepath.Join(t.TempDir(), "out.pem"), bad)

	assert.Error(t, err)
}
