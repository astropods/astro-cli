// Package hostproxy carries the developer's proxy environment into dev
// containers: the proxy URLs, their NO_PROXY list, and the CAs they need trusted.
package hostproxy

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/astropods/astro-cli/internal/compose"
	"github.com/astropods/astro-cli/internal/envvar"
)

const (
	dockerHostGateway = "host.docker.internal"
	caBundleMountPath = "/etc/astro/proxy/ca-bundle.pem"
)

// ErrLoopbackOnLinux: host.docker.internal can't reach a host-loopback listener on Linux.
var ErrLoopbackOnLinux = errors.New("a proxy on a loopback address isn't reachable from containers on Linux")

// InvalidProxyError reports a proxy variable whose value isn't a usable URL.
// It never carries the value, which may hold credentials.
type InvalidProxyError struct{ Var string }

func (e *InvalidProxyError) Error() string { return e.Var + " is not a valid proxy URL" }

// CAFileError reports a CA file named in the environment that can't be used.
type CAFileError struct {
	Var  string
	Path string
	Err  error
}

func (e *CAFileError) Error() string { return fmt.Sprintf("%s=%s: %v", e.Var, e.Path, e.Err) }
func (e *CAFileError) Unwrap() error { return e.Err }

// Proxy is one proxy URL from the host environment.
type Proxy struct {
	// Var is the variable it came from, e.g. HTTPS_PROXY.
	Var string
	// HostURL is the proxy as the host reaches it.
	HostURL *url.URL
	// ContainerURL is the proxy as containers reach it.
	ContainerURL *url.URL
	Loopback     bool
}

// DisplayHostURL is HostURL with any password redacted.
func (p *Proxy) DisplayHostURL() string { return p.HostURL.Redacted() }

// DisplayContainerURL is ContainerURL with any password redacted.
func (p *Proxy) DisplayContainerURL() string { return p.ContainerURL.Redacted() }

// Config is the host's proxy environment, as the containers need it.
type Config struct {
	HTTPS   *Proxy // from HTTPS_PROXY or https_proxy
	HTTP    *Proxy // from HTTP_PROXY or http_proxy
	NoProxy []string
	CAFiles map[string]string // env var -> path
}

// Proxies returns the configured proxies, HTTPS first.
func (c *Config) Proxies() []*Proxy {
	var out []*Proxy
	for _, p := range []*Proxy{c.HTTPS, c.HTTP} {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}

var caVars = []string{"SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE"}

// FromEnv reads the proxy environment through getenv. It returns nil when no
// proxy is set or envvar.NoHostProxy is set. Each proxy is carried as the host
// has it: an HTTPS proxy alone doesn't become the HTTP proxy.
func FromEnv(getenv func(string) string, goos string) (*Config, error) {
	if v := getenv(envvar.NoHostProxy); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return nil, nil
	}
	cfg := &Config{CAFiles: map[string]string{}}
	var err error
	if cfg.HTTPS, err = proxyFromEnv(getenv, goos, "HTTPS_PROXY", "https_proxy"); err != nil {
		return nil, err
	}
	if cfg.HTTP, err = proxyFromEnv(getenv, goos, "HTTP_PROXY", "http_proxy"); err != nil {
		return nil, err
	}
	if cfg.HTTPS == nil && cfg.HTTP == nil {
		return nil, nil
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

func proxyFromEnv(getenv func(string) string, goos string, vars ...string) (*Proxy, error) {
	for _, k := range vars {
		raw := strings.TrimSpace(getenv(k))
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return nil, &InvalidProxyError{Var: k}
		}
		p := &Proxy{Var: k, HostURL: u, ContainerURL: u}
		if isLoopback(u.Hostname()) {
			if goos == "linux" {
				return nil, ErrLoopbackOnLinux
			}
			c := *u
			if port := u.Port(); port != "" {
				c.Host = net.JoinHostPort(dockerHostGateway, port)
			} else {
				c.Host = dockerHostGateway
			}
			p.ContainerURL, p.Loopback = &c, true
		}
		return p, nil
	}
	return nil, nil
}

// Check returns the first proxy that doesn't accept a TCP connection from the
// host, with the dial error.
func (c *Config) Check(ctx context.Context, dial func(ctx context.Context, network, addr string) (net.Conn, error)) (*Proxy, error) {
	for _, p := range c.Proxies() {
		port := p.HostURL.Port()
		if port == "" {
			port = "80"
			if p.HostURL.Scheme == "https" {
				port = "443"
			}
		}
		conn, err := dial(ctx, "tcp", net.JoinHostPort(p.HostURL.Hostname(), port))
		if err != nil {
			return p, err
		}
		_ = conn.Close()
	}
	return nil, nil
}

// Runtime validates the CA files, writes the combined bundle into bundleDir,
// and returns what the containers need. With no CA files in the environment,
// it adds no bundle.
func (c *Config) Runtime(bundleDir string) (*compose.Runtime, error) {
	rt := &compose.Runtime{
		Env: map[string]string{"NODE_USE_ENV_PROXY": "1"},
		// host.docker.internal keeps calls to host-local services (Ollama, a local MCP server) off the proxy.
		NoProxy: append([]string{"localhost", "127.0.0.1", dockerHostGateway}, c.NoProxy...),
	}
	if c.HTTPS != nil {
		rt.Env["HTTPS_PROXY"] = c.HTTPS.ContainerURL.String()
		rt.Env["https_proxy"] = c.HTTPS.ContainerURL.String()
	}
	if c.HTTP != nil {
		rt.Env["HTTP_PROXY"] = c.HTTP.ContainerURL.String()
		rt.Env["http_proxy"] = c.HTTP.ContainerURL.String()
	}
	for _, p := range c.Proxies() {
		if p.Loopback {
			rt.ExtraHosts = []string{dockerHostGateway + ":host-gateway"}
		}
	}
	if len(c.CAFiles) == 0 {
		return rt, nil
	}

	var seen []string
	var certs [][]byte
	for _, k := range caVars {
		p, ok := c.CAFiles[k]
		if !ok || slices.Contains(seen, p) {
			continue
		}
		seen = append(seen, p)
		ders, err := readCertificates(p)
		if err != nil {
			return nil, &CAFileError{Var: k, Path: p, Err: err}
		}
		certs = append(certs, ders...)
	}
	bundle, err := WriteCABundle(bundleDir, certs)
	if err != nil {
		return nil, err
	}
	rt.Mounts = []compose.Mount{{Source: bundle, Target: caBundleMountPath}}
	for _, k := range caVars {
		rt.Env[k] = caBundleMountPath
	}
	return rt, nil
}

// readCertificates returns the DER of every PEM block in path. It fails on a
// file with no blocks, or with any block that isn't a certificate.
func readCertificates(path string) ([][]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // CA path from the developer's environment
	if err != nil {
		return nil, err
	}
	var ders [][]byte
	for block, rest := pem.Decode(data); block != nil; block, rest = pem.Decode(rest) {
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("holds a %s block, not only certificates", block.Type)
		}
		ders = append(ders, block.Bytes)
	}
	if len(ders) == 0 {
		return nil, errors.New("holds no PEM certificate")
	}
	return ders, nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
