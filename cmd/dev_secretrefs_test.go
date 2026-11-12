package cmd

import (
	"bytes"
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

	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	composeBuilder "github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/secretref"
)

const devSecretTestPlaceholder = "{{vault:856c3944-0ba6-5de7-a9ea-f85975c4453a}}"

func devSecretTestSpec() *spec.AstroSpec {
	return &spec.AstroSpec{
		Name:   "passport-agent",
		Agent:  spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{"anthropic": {Provider: "anthropic"}},
	}
}

func writeDevSecretTestCA(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "passport-test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
}

// usePassportForTest installs a Passport provider whose `passport daemon status --json` prints status.
func usePassportForTest(t *testing.T, status string, runErr error) {
	t.Helper()
	passportDir := t.TempDir()
	writeDevSecretTestCA(t, passportDir)
	devSecretProvidersOverride = []secretref.Provider{&secretref.Passport{
		PassportDir: passportDir,
		BundleDir:   t.TempDir(),
		LookPath:    func(string) (string, error) { return "/usr/local/bin/passport", nil },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(status), runErr
		},
	}}
	t.Cleanup(func() { devSecretProvidersOverride = nil })
}

func TestBuildDevComposeProject_PassportPlaceholderRoutesTheAgent(t *testing.T) {
	usePassportForTest(t, `{"running":true,"port":10144,"upstream":{"reachable":true}}`, nil)
	t.Setenv("HTTPS_PROXY", "")
	var out bytes.Buffer

	project, err := buildDevComposeProject(context.Background(), &out, devSecretTestSpec(), t.TempDir(),
		map[string]string{"ANTHROPIC_API_KEY": devSecretTestPlaceholder})

	require.NoError(t, err)
	env := project.Services["agent"].Environment
	require.NotNil(t, env["ANTHROPIC_API_KEY"])
	assert.Equal(t, devSecretTestPlaceholder, *env["ANTHROPIC_API_KEY"], "the agent holds only the placeholder")
	require.NotNil(t, env["HTTPS_PROXY"], "the agent is routed through the Passport daemon")
	assert.Equal(t, "http://host.docker.internal:10144", *env["HTTPS_PROXY"])
	var targets []string
	for _, v := range project.Services["agent"].Volumes {
		targets = append(targets, v.Target)
	}
	assert.Contains(t, targets, "/etc/astro/passport/ca-bundle.pem", "the CA bundle is mounted into the agent")
	assert.Empty(t, os.Getenv("HTTPS_PROXY"), "the proxy never reaches the CLI process, so image builds don't use it")
	assert.Equal(t, msgSecretRefsResolved(secretref.Use{Provider: "Passport", Keys: []string{"ANTHROPIC_API_KEY"}, Detail: "via daemon on :10144"}), out.String())
}

func TestBuildDevComposeProject_NoReferencesMatchesBuildProject(t *testing.T) {
	devSecretProvidersOverride = []secretref.Provider{&secretref.Passport{
		LookPath: func(string) (string, error) { return "", errors.New("must not be called") },
	}}
	t.Cleanup(func() { devSecretProvidersOverride = nil })
	dir := t.TempDir()
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-plain"}
	want, err := composeBuilder.BuildProject(devSecretTestSpec(), dir, env)
	require.NoError(t, err)
	var out bytes.Buffer

	got, err := buildDevComposeProject(context.Background(), &out, devSecretTestSpec(), dir, env)

	require.NoError(t, err, "a project without references never checks Passport")
	assert.Equal(t, want, got, "without references the project is exactly what BuildProject produces")
	assert.Empty(t, out.String())
}

func TestBuildDevComposeProject_FailedCheckStopsWithItsFix(t *testing.T) {
	usePassportForTest(t, `{"running":false}`, nil)

	_, err := buildDevComposeProject(context.Background(), &bytes.Buffer{}, devSecretTestSpec(), t.TempDir(),
		map[string]string{"ANTHROPIC_API_KEY": devSecretTestPlaceholder})

	want := errSecretRefCheck(&secretref.CheckError{Failures: map[string]error{"Passport": secretref.ErrPassportDaemonStopped}})
	require.Error(t, err)
	assert.Equal(t, want.Error(), err.Error())
}

func TestStoredProxyEnvKeys(t *testing.T) {
	got := storedProxyEnvKeys(map[string]string{
		"HTTPS_PROXY": "x", "no_proxy": "y", "SSL_CERT_FILE": "z", "ANTHROPIC_API_KEY": "k", "MY_HTTPS_PROXY_NOTE": "n",
	})

	assert.Equal(t, []string{"HTTPS_PROXY", "SSL_CERT_FILE", "no_proxy"}, got, "matching is case-insensitive and exact on the key name")
}

func TestMsgSecretRefFix(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{secretref.ErrPassportNotInstalled, "the passport CLI isn't installed. Install the Passport CLI Enterprise Edition, then run 'passport setup'."},
		{secretref.ErrPassportDaemonStopped, "the Passport daemon isn't running. Run 'passport daemon start'."},
		{secretref.ErrPassportUpstream, "the Passport daemon can't reach your Passport proxy. Check that the proxy is up, then run 'passport daemon status'."},
		{secretref.ErrPassportCANotFound, "the Passport CA certificate is missing. Run 'passport setup'."},
		{&secretref.PassportMintError{Detail: "ETIMEDOUT"}, "the Passport daemon couldn't renew its certificate (ETIMEDOUT). It retries on the next request; if this keeps failing, run 'passport setup'."},
	}
	for _, tc := range cases {
		t.Run(tc.want[:20], func(t *testing.T) {
			assert.Equal(t, tc.want, msgSecretRefFix(tc.err))
		})
	}
}
