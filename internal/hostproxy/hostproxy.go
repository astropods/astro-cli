// Package hostproxy carries the developer's proxy environment into dev
// containers: the proxy URL, its NO_PROXY list, and the CAs it needs trusted.
package hostproxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/envvar"
)

const (
	dockerHostGateway = "host.docker.internal"
	caBundleMountPath = "/etc/astro/proxy/ca-bundle.pem"
)

var (
	// ErrLoopbackOnLinux: host.docker.internal can't reach a host-loopback listener on Linux.
	ErrLoopbackOnLinux = errors.New("a proxy on a loopback address isn't reachable from containers on Linux")
	ErrInvalidProxyURL = errors.New("proxy URL is not valid")
)

// CAFileError reports a CA file named in the environment that can't be used.
type CAFileError struct {
	Var  string
	Path string
	Err  error
}

func (e *CAFileError) Error() string { return fmt.Sprintf("%s=%s: %v", e.Var, e.Path, e.Err) }
func (e *CAFileError) Unwrap() error { return e.Err }

// Config is the host's proxy environment, as the containers need it.
type Config struct {
	// HostURL is the proxy as the host reaches it, e.g. http://127.0.0.1:10144.
	HostURL string
	// ContainerURL is the proxy as containers reach it.
	ContainerURL string
	Loopback     bool
	NoProxy      []string
	CAFiles      map[string]string // env var -> path
}

var proxyVars = []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"}

var caVars = []string{"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE"}

// FromEnv reads the proxy environment through getenv. It returns nil when no
// proxy is set or envvar.NoHostProxy is set.
func FromEnv(getenv func(string) string, goos string) (*Config, error) {
	if v := getenv(envvar.NoHostProxy); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return nil, nil
	}
	raw := ""
	for _, k := range proxyVars {
		if raw = strings.TrimSpace(getenv(k)); raw != "" {
			break
		}
	}
	if raw == "" {
		return nil, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProxyURL, raw)
	}

	cfg := &Config{HostURL: u.String(), ContainerURL: u.String(), CAFiles: map[string]string{}}
	if isLoopback(u.Hostname()) {
		if goos == "linux" {
			return nil, ErrLoopbackOnLinux
		}
		cfg.Loopback = true
		c := *u
		if port := u.Port(); port != "" {
			c.Host = net.JoinHostPort(dockerHostGateway, port)
		} else {
			c.Host = dockerHostGateway
		}
		cfg.ContainerURL = c.String()
	}

	noProxy := getenv("NO_PROXY")
	if noProxy == "" {
		noProxy = getenv("no_proxy")
	}
	for _, h := range strings.Split(noProxy, ",") {
		if h = strings.TrimSpace(h); h != "" {
			cfg.NoProxy = append(cfg.NoProxy, h)
		}
	}

	for _, k := range caVars {
		if p := strings.TrimSpace(getenv(k)); p != "" {
			cfg.CAFiles[k] = p
		}
	}
	return cfg, nil
}

// Check fails when the proxy doesn't accept a TCP connection from the host.
func (c *Config) Check(ctx context.Context, dial func(ctx context.Context, network, addr string) (net.Conn, error)) error {
	u, err := url.Parse(c.HostURL)
	if err != nil {
		return err
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443"}[u.Scheme]
		if port == "" {
			port = "80"
		}
	}
	conn, err := dial(ctx, "tcp", net.JoinHostPort(u.Hostname(), port))
	if err != nil {
		return err
	}
	return conn.Close()
}

// Runtime writes the combined CA bundle under bundleDir and returns what the
// containers need. With no CA files in the environment, it adds no bundle.
func (c *Config) Runtime(bundleDir string) (*compose.Runtime, error) {
	rt := &compose.Runtime{
		Env: map[string]string{
			"HTTPS_PROXY":        c.ContainerURL,
			"https_proxy":        c.ContainerURL,
			"HTTP_PROXY":         c.ContainerURL,
			"http_proxy":         c.ContainerURL,
			"NODE_USE_ENV_PROXY": "1",
		},
		NoProxy: append([]string{"localhost", "127.0.0.1"}, c.NoProxy...),
	}
	if c.Loopback {
		rt.ExtraHosts = []string{dockerHostGateway + ":host-gateway"}
	}
	if len(c.CAFiles) == 0 {
		return rt, nil
	}

	var paths []string
	for _, k := range caVars {
		p, ok := c.CAFiles[k]
		if !ok || slices.Contains(paths, p) {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			return nil, &CAFileError{Var: k, Path: p, Err: err}
		}
		paths = append(paths, p)
	}
	bundle, err := WriteCABundle(filepath.Join(bundleDir, "ca-bundle.pem"), paths...)
	if err != nil {
		return nil, err
	}
	rt.Mounts = []compose.Mount{{Source: bundle, Target: caBundleMountPath}}
	for _, k := range caVars {
		rt.Env[k] = caBundleMountPath
	}
	return rt, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
