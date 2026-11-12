package secretref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
)

// Passport errors, mapped to user-facing messages by the caller.
var (
	ErrPassportNotInstalled  = errors.New("passport CLI not found on PATH")
	ErrPassportDaemonStopped = errors.New("passport daemon is not running")
	ErrPassportUpstream      = errors.New("passport proxy is unreachable")
	ErrPassportCANotFound    = errors.New("passport CA certificate not found")
	// Containers reach the daemon through host.docker.internal, which on Linux
	// can't reach the daemon's 127.0.0.1 listener.
	ErrPassportUnsupportedPlatform = errors.New("passport routing is not supported on linux")
)

// PassportMintError reports the daemon's last failed certificate mint.
type PassportMintError struct{ Detail string }

func (e *PassportMintError) Error() string { return "passport certificate mint failed: " + e.Detail }

// The proxy only substitutes a token that is exactly a UUID.
var passportPlaceholder = regexp.MustCompile(`\{\{vault:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\}\}`)

const (
	dockerHostGateway    = "host.docker.internal"
	caBundleMountPath    = "/etc/astro/passport/ca-bundle.pem"
	passportDefaultDir   = ".passport"
	passportBundleSubdir = "passport"
)

// CommandRunner runs a command and returns its stdout.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Passport routes container traffic through the local Passport daemon, which
// swaps {{vault:<uuid>}} placeholders for real secrets at request time.
type Passport struct {
	// BundleDir is where the combined CA bundle is written, e.g. ~/.ast/passport.
	BundleDir string
	// Run defaults to exec; tests replace it.
	Run CommandRunner
	// LookPath defaults to exec.LookPath; tests replace it.
	LookPath func(string) (string, error)
	// PassportDir defaults to $PASSPORT_DIR, then ~/.passport.
	PassportDir string
	// GOOS defaults to runtime.GOOS; tests replace it.
	GOOS string

	status *passportStatus
}

type passportStatus struct {
	Running  bool `json:"running"`
	Port     int  `json:"port"`
	Upstream struct {
		Reachable bool `json:"reachable"`
	} `json:"upstream"`
	LastMintError *string `json:"lastMintError"`
}

func (p *Passport) Name() string { return "Passport" }

func (p *Passport) Matches(value string) bool { return passportPlaceholder.MatchString(value) }

// Check needs the daemon running with a reachable upstream. An expired client
// cert is fine: the daemon mints a new one on the next request.
func (p *Passport) Check(ctx context.Context) error {
	goos := p.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "linux" {
		return ErrPassportUnsupportedPlatform
	}
	lookPath := p.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if _, err := lookPath("passport"); err != nil {
		return ErrPassportNotInstalled
	}
	out, err := p.run(ctx, "passport", "daemon", "status", "--json")
	if err != nil {
		return ErrPassportDaemonStopped
	}
	var st passportStatus
	if err := json.Unmarshal(out, &st); err != nil {
		return fmt.Errorf("parse passport daemon status: %w", err)
	}
	switch {
	case !st.Running || st.Port == 0:
		return ErrPassportDaemonStopped
	case !st.Upstream.Reachable:
		return ErrPassportUpstream
	case st.LastMintError != nil && *st.LastMintError != "":
		return &PassportMintError{Detail: *st.LastMintError}
	}
	if _, err := os.Stat(p.caPath()); err != nil {
		return ErrPassportCANotFound
	}
	p.status = &st
	return nil
}

// Resolve leaves placeholders as they are: the daemon replaces them in flight.
// It writes the CA bundle and returns the container routing.
func (p *Passport) Resolve(_ context.Context, env map[string]string) (Resolution, error) {
	if p.status == nil {
		return Resolution{}, errors.New("passport: Resolve called before a successful Check")
	}
	bundle, err := WriteCABundle(filepath.Join(p.BundleDir, "ca-bundle.pem"), p.caPath())
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{
		Env: env,
		Runtime: &ContainerRuntime{
			Env: map[string]string{
				"NODE_EXTRA_CA_CERTS": caBundleMountPath,
				"SSL_CERT_FILE":       caBundleMountPath,
				"REQUESTS_CA_BUNDLE":  caBundleMountPath,
				"NODE_USE_ENV_PROXY":  "1",
			},
			Mounts:     []Mount{{Source: bundle, Target: caBundleMountPath}},
			ExtraHosts: []string{dockerHostGateway + ":host-gateway"},
			Proxy: &Proxy{
				URL:     fmt.Sprintf("http://%s:%d", dockerHostGateway, p.status.Port),
				NoProxy: []string{"localhost", "127.0.0.1"},
			},
		},
		Detail: fmt.Sprintf("via daemon on :%d", p.status.Port),
	}, nil
}

func (p *Passport) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if p.Run != nil {
		return p.Run(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).Output() //nolint:gosec // fixed binary name and args
}

func (p *Passport) caPath() string {
	dir := p.PassportDir
	if dir == "" {
		dir = os.Getenv("PASSPORT_DIR")
	}
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, passportDefaultDir)
		}
	}
	return filepath.Join(dir, "ca.pem")
}

// PassportBundleDir returns the bundle directory under the CLI's config dir.
func PassportBundleDir(configDir string) string {
	return filepath.Join(configDir, passportBundleSubdir)
}
