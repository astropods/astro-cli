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
	type want struct {
		v, host, inside string
		loopback        bool
	}
	cases := []struct {
		name        string
		env         map[string]string
		goos        string
		wantNil     bool
		wantErr     error
		wantInvalid string
		https, http *want
		wantNoProxy []string
	}{
		{name: "no proxy set", env: map[string]string{}, wantNil: true},
		{name: "opted out", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", envvar.NoHostProxy: "1"}, wantNil: true},
		{name: "opt-out set to 0 keeps the proxy", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", envvar.NoHostProxy: "0"},
			https: &want{"HTTPS_PROXY", "http://127.0.0.1:10144", "http://host.docker.internal:10144", true}},
		{name: "127.0.0.1 is rewritten for containers", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", "NO_PROXY": "localhost, .internal ,"},
			https: &want{"HTTPS_PROXY", "http://127.0.0.1:10144", "http://host.docker.internal:10144", true}, wantNoProxy: []string{"localhost", ".internal"}},
		{name: "lowercase https_proxy counts", env: map[string]string{"https_proxy": "http://localhost:3128"},
			https: &want{"https_proxy", "http://localhost:3128", "http://host.docker.internal:3128", true}},
		{name: "IPv6 loopback is rewritten", env: map[string]string{"HTTPS_PROXY": "http://[::1]:3128"},
			https: &want{"HTTPS_PROXY", "http://[::1]:3128", "http://host.docker.internal:3128", true}},
		{name: "remote proxy is used as is", env: map[string]string{"HTTPS_PROXY": "http://proxy.corp.example:3128"},
			https: &want{"HTTPS_PROXY", "http://proxy.corp.example:3128", "http://proxy.corp.example:3128", false}},
		{name: "HTTPS and HTTP are carried separately", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", "HTTP_PROXY": "http://proxy.corp.example:3128"},
			https: &want{"HTTPS_PROXY", "http://127.0.0.1:10144", "http://host.docker.internal:10144", true},
			http:  &want{"HTTP_PROXY", "http://proxy.corp.example:3128", "http://proxy.corp.example:3128", false}},
		{name: "HTTP alone doesn't become the HTTPS proxy", env: map[string]string{"http_proxy": "http://proxy.corp.example:3128"},
			http: &want{"http_proxy", "http://proxy.corp.example:3128", "http://proxy.corp.example:3128", false}},
		{name: "a URL without a scheme gets http", env: map[string]string{"HTTPS_PROXY": "proxy.corp.example:3128"},
			https: &want{"HTTPS_PROXY", "http://proxy.corp.example:3128", "http://proxy.corp.example:3128", false}},
		{name: "loopback on linux is refused", env: map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144"}, goos: "linux", wantErr: ErrLoopbackOnLinux},
		{name: "remote proxy on linux is fine", env: map[string]string{"HTTPS_PROXY": "http://proxy.corp.example:3128"}, goos: "linux",
			https: &want{"HTTPS_PROXY", "http://proxy.corp.example:3128", "http://proxy.corp.example:3128", false}},
		{name: "an invalid HTTP_PROXY names its variable", env: map[string]string{"HTTPS_PROXY": "http://proxy.corp.example:3128", "HTTP_PROXY": "http://:3128"}, wantInvalid: "HTTP_PROXY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goos := tc.goos
			if goos == "" {
				goos = "darwin"
			}
			cfg, err := FromEnv(envOf(tc.env), goos)
			switch {
			case tc.wantErr != nil:
				assert.ErrorIs(t, err, tc.wantErr)
				return
			case tc.wantInvalid != "":
				var invalid *InvalidProxyError
				require.ErrorAs(t, err, &invalid)
				assert.Equal(t, tc.wantInvalid, invalid.Var)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, cfg)
				return
			}
			require.NotNil(t, cfg)
			check := func(label string, got *Proxy, w *want) {
				if w == nil {
					assert.Nil(t, got, "no %s proxy", label)
					return
				}
				require.NotNil(t, got, "%s proxy", label)
				assert.Equal(t, w.v, got.Var)
				assert.Equal(t, w.host, got.HostURL.String())
				assert.Equal(t, w.inside, got.ContainerURL.String())
				assert.Equal(t, w.loopback, got.Loopback)
			}
			check("HTTPS", cfg.HTTPS, tc.https)
			check("HTTP", cfg.HTTP, tc.http)
			assert.Equal(t, tc.wantNoProxy, cfg.NoProxy)
		})
	}
}

func TestProxy_DisplayRedactsThePassword(t *testing.T) {
	cfg, err := FromEnv(envOf(map[string]string{"HTTPS_PROXY": "http://alice:s3cret@127.0.0.1:3128"}), "darwin")
	require.NoError(t, err)

	assert.NotContains(t, cfg.HTTPS.DisplayHostURL(), "s3cret")
	assert.NotContains(t, cfg.HTTPS.DisplayContainerURL(), "s3cret")
	assert.Equal(t, "http://alice:s3cret@host.docker.internal:3128", cfg.HTTPS.ContainerURL.String(), "the container still gets the credentials")
}

func TestInvalidProxyError_NeverEchoesTheValue(t *testing.T) {
	_, err := FromEnv(envOf(map[string]string{"HTTPS_PROXY": "http://alice:s3cret@:3128"}), "darwin")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret")
}

func TestCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	dial := (&net.Dialer{Timeout: time.Second}).DialContext
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	downAddr := closed.Addr().String()
	require.NoError(t, closed.Close())

	up, err := FromEnv(envOf(map[string]string{"HTTPS_PROXY": "http://" + ln.Addr().String()}), "darwin")
	require.NoError(t, err)
	failed, err := up.Check(context.Background(), dial)
	assert.NoError(t, err, "a listening proxy passes")
	assert.Nil(t, failed)

	mixed, err := FromEnv(envOf(map[string]string{"HTTPS_PROXY": "http://" + ln.Addr().String(), "HTTP_PROXY": "http://" + downAddr}), "darwin")
	require.NoError(t, err)
	failed, err = mixed.Check(context.Background(), dial)
	assert.Error(t, err, "every configured proxy is checked")
	require.NotNil(t, failed)
	assert.Equal(t, "HTTP_PROXY", failed.Var, "the failure names the proxy that isn't answering")
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

func testConfig(t *testing.T, env map[string]string) *Config {
	t.Helper()
	cfg, err := FromEnv(envOf(env), "darwin")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	return cfg
}

func TestRuntime_RoutesAndTrustsTheHostCAs(t *testing.T) {
	ca := writeTestCA(t, t.TempDir(), "passport-proxy")
	bundleDir := t.TempDir()
	cfg := testConfig(t, map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", "NO_PROXY": ".internal", "SSL_CERT_FILE": ca, "NODE_EXTRA_CA_CERTS": ca})

	rt, err := cfg.Runtime(bundleDir)

	require.NoError(t, err)
	assert.Equal(t, "http://host.docker.internal:10144", rt.Env["HTTPS_PROXY"])
	assert.Equal(t, "http://host.docker.internal:10144", rt.Env["https_proxy"])
	assert.NotContains(t, rt.Env, "HTTP_PROXY", "the host set no HTTP proxy, so the containers get none")
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
		require.Equal(t, "CERTIFICATE", block.Type)
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

func TestRuntime_SeparateHTTPAndHTTPSProxies(t *testing.T) {
	cfg := testConfig(t, map[string]string{"HTTPS_PROXY": "http://127.0.0.1:10144", "HTTP_PROXY": "http://proxy.corp.example:3128"})

	rt, err := cfg.Runtime(t.TempDir())

	require.NoError(t, err)
	assert.Equal(t, "http://host.docker.internal:10144", rt.Env["HTTPS_PROXY"])
	assert.Equal(t, "http://proxy.corp.example:3128", rt.Env["HTTP_PROXY"])
	assert.Equal(t, "http://proxy.corp.example:3128", rt.Env["http_proxy"])
}

func TestRuntime_NoCAFilesMeansNoBundle(t *testing.T) {
	cfg := testConfig(t, map[string]string{"HTTPS_PROXY": "http://proxy.corp.example:3128"})

	rt, err := cfg.Runtime(t.TempDir())

	require.NoError(t, err)
	assert.Empty(t, rt.Mounts)
	assert.Empty(t, rt.ExtraHosts, "a remote proxy needs no host mapping")
	assert.NotContains(t, rt.Env, "SSL_CERT_FILE", "the container keeps its own trust store")
}

func TestRuntime_UnusableCAFile(t *testing.T) {
	dir := t.TempDir()
	cert := writeTestCA(t, dir, "ok")
	certPEM, err := os.ReadFile(cert)
	require.NoError(t, err)
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, data, 0o600))
		return p
	}
	cases := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join(dir, "missing.pem")},
		{"not PEM", write("garbage.pem", []byte("not a cert"))},
		{"private key", write("key.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")}))},
		{"certificate followed by a private key", write("mixed.pem", append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})...))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(t, map[string]string{"HTTPS_PROXY": "http://127.0.0.1:1", "SSL_CERT_FILE": tc.path})

			_, err := cfg.Runtime(t.TempDir())

			var caErr *CAFileError
			require.ErrorAs(t, err, &caErr)
			assert.Equal(t, "SSL_CERT_FILE", caErr.Var)
			assert.Equal(t, tc.path, caErr.Path)
		})
	}
}
