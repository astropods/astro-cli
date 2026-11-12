package secretref

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testUUID = "856c3944-0ba6-5de7-a9ea-f85975c4453a"

func TestPassport_Matches(t *testing.T) {
	p := &Passport{}
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"whole value", "{{vault:" + testUUID + "}}", true},
		{"inside a value", "Bearer {{vault:" + testUUID + "}}", true},
		{"uppercase hex", "{{vault:856C3944-0BA6-5DE7-A9EA-F85975C4453A}}", true},
		{"name instead of uuid", "{{vault:anthropic-api-key}}", false},
		{"truncated uuid", "{{vault:856c3944-0ba6-5de7}}", false},
		{"plain value", "sk-ant-123", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.Matches(tc.value))
		})
	}
}

func writeTestCA(t *testing.T, dir string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "passport-proxy-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	path := filepath.Join(dir, "ca.pem")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	return path
}

func fakeRunner(out string, err error) CommandRunner {
	return func(context.Context, string, ...string) ([]byte, error) { return []byte(out), err }
}

func installed(string) (string, error) { return "/usr/local/bin/passport", nil }

func TestPassport_Check(t *testing.T) {
	healthy := `{"running":true,"port":10144,"upstream":{"reachable":true},"lastMintError":null,"certExpiresInSec":-62}`
	cases := []struct {
		name     string
		lookPath func(string) (string, error)
		out      string
		runErr   error
		noCA     bool
		wantErr  error
		wantMint bool
	}{
		{name: "healthy daemon with an expired cert passes", lookPath: installed, out: healthy},
		{name: "binary missing", lookPath: func(string) (string, error) { return "", errors.New("nope") }, wantErr: ErrPassportNotInstalled},
		{name: "status command fails", lookPath: installed, runErr: errors.New("exit 1"), wantErr: ErrPassportDaemonStopped},
		{name: "daemon not running", lookPath: installed, out: `{"running":false}`, wantErr: ErrPassportDaemonStopped},
		{name: "upstream unreachable", lookPath: installed, out: `{"running":true,"port":10144,"upstream":{"reachable":false}}`, wantErr: ErrPassportUpstream},
		{name: "last mint failed", lookPath: installed, out: `{"running":true,"port":10144,"upstream":{"reachable":true},"lastMintError":"ETIMEDOUT"}`, wantMint: true},
		{name: "CA missing", lookPath: installed, out: healthy, noCA: true, wantErr: ErrPassportCANotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if !tc.noCA {
				writeTestCA(t, dir)
			}
			p := &Passport{PassportDir: dir, LookPath: tc.lookPath, Run: fakeRunner(tc.out, tc.runErr)}

			err := p.Check(context.Background())

			switch {
			case tc.wantMint:
				var mint *PassportMintError
				require.ErrorAs(t, err, &mint)
				assert.Equal(t, "ETIMEDOUT", mint.Detail)
			case tc.wantErr != nil:
				assert.ErrorIs(t, err, tc.wantErr)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

func TestPassport_ResolveBeforeCheckFails(t *testing.T) {
	_, err := (&Passport{}).Resolve(context.Background(), map[string]string{"K": "{{vault:" + testUUID + "}}"})
	assert.Error(t, err, "Resolve needs the daemon port that only a successful Check provides")
}

func TestPassport_ResolveRoutesThroughDaemonAndKeepsPlaceholders(t *testing.T) {
	passportDir := t.TempDir()
	writeTestCA(t, passportDir)
	bundleDir := filepath.Join(t.TempDir(), "passport")
	p := &Passport{
		PassportDir: passportDir,
		BundleDir:   bundleDir,
		LookPath:    installed,
		Run:         fakeRunner(`{"running":true,"port":10155,"upstream":{"reachable":true}}`, nil),
	}
	env := map[string]string{"ANTHROPIC_API_KEY": "{{vault:" + testUUID + "}}"}
	require.NoError(t, p.Check(context.Background()))

	res, err := p.Resolve(context.Background(), env)

	require.NoError(t, err)
	assert.Equal(t, env, res.Env, "placeholders reach the container as-is; the daemon replaces them in flight")
	assert.Equal(t, "via daemon on :10155", res.Detail)
	require.NotNil(t, res.Runtime.Proxy)
	assert.Equal(t, "http://host.docker.internal:10155", res.Runtime.Proxy.URL, "the proxy URL uses the daemon's reported port")
	assert.Equal(t, []string{"host.docker.internal:host-gateway"}, res.Runtime.ExtraHosts)
	assert.Equal(t, "1", res.Runtime.Env["NODE_USE_ENV_PROXY"], "Node 24 bypasses HTTPS_PROXY without it")
	for _, k := range []string{"NODE_EXTRA_CA_CERTS", "SSL_CERT_FILE", "REQUESTS_CA_BUNDLE"} {
		assert.Equal(t, caBundleMountPath, res.Runtime.Env[k], "%s points at the mounted combined bundle", k)
	}
	require.Len(t, res.Runtime.Mounts, 1)
	assert.Equal(t, filepath.Join(bundleDir, "ca-bundle.pem"), res.Runtime.Mounts[0].Source)
	assert.Equal(t, caBundleMountPath, res.Runtime.Mounts[0].Target)
	assert.FileExists(t, res.Runtime.Mounts[0].Source)
}
