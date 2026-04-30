package k8s

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrImageNotFound is returned by ImagePreflighter when the registry reports
// that the image manifest does not exist. Distinct from a transport error so
// callers can fail fast (HTTP 422) instead of letting kubelet discover the
// missing tag 35 minutes later as ImagePullBackOff.
type ErrImageNotFound struct {
	// Image is the fully resolved image reference that was checked
	// (e.g. "registry.localhost/team/agent-name:b7396c13").
	Image string
	// BuildID, when non-empty, is propagated in the HTTP error body so the
	// CLI/UI can surface "build X is gone" to the user.
	BuildID string
	// Reason describes why the image was treated as not found
	// ("404", "local mirror 500", etc.) for observability only.
	Reason string
}

func (e *ErrImageNotFound) Error() string {
	if e == nil {
		return "image not found"
	}
	if e.Reason == "" {
		return fmt.Sprintf("image not found in registry: %s", e.Image)
	}
	return fmt.Sprintf("image not found in registry: %s (%s)", e.Image, e.Reason)
}

// AsImageNotFound unwraps err to *ErrImageNotFound; ok=false if err does not
// carry one. Use instead of errors.As when callers want both the typed value
// and a quick boolean check.
func AsImageNotFound(err error) (*ErrImageNotFound, bool) {
	var target *ErrImageNotFound
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// ImagePreflighter performs a registry HEAD against an image manifest before
// the deploy job is enqueued. A preflight that returns ErrImageNotFound short-
// circuits the deploy with a fast 422; any other outcome is fail-open (the
// deploy proceeds, kubelet will surface real pull errors).
//
// A positive (200) response is cached for `ttl` to avoid hammering the
// registry across multi-component deploys that share an image (e.g. the
// agent image referenced by both the agent and a sidecar).
type ImagePreflighter struct {
	client http.Client
	ttl    time.Duration
	cache  sync.Map // image string -> time.Time (cache expiry)

	// localMode treats 5xx responses from the local registry mirror as
	// "not found". The local astro-registry returns 500 for missing tags
	// rather than the standard 404, which is the canonical case behind
	// the 35-minute ImagePullBackOff bug.
	localMode bool
	// localMirrorHosts (when localMode) restricts the 5xx-as-404 special case
	// to specific hosts. An empty slice means "any host while in local mode".
	localMirrorHosts []string
}

// NewImagePreflighter constructs a preflighter with sensible defaults
// (5s HTTP timeout, 60s positive-result cache).
func NewImagePreflighter(localMode bool, localMirrorHosts ...string) *ImagePreflighter {
	return &ImagePreflighter{
		client:           http.Client{Timeout: 5 * time.Second},
		ttl:              60 * time.Second,
		localMode:        localMode,
		localMirrorHosts: localMirrorHosts,
	}
}

// SetTTL overrides the positive-result cache duration. Intended for tests.
func (p *ImagePreflighter) SetTTL(d time.Duration) {
	if p == nil {
		return
	}
	p.ttl = d
}

// SetClient overrides the HTTP client. Intended for tests that need to plug
// in an httptest.Server.
func (p *ImagePreflighter) SetClient(c http.Client) {
	if p == nil {
		return
	}
	p.client = c
}

// Preflight checks whether image exists in the registry. Returns:
//   - nil on 200 (cached for ttl), unrecognized formats (fail open), and
//     transport errors (fail open: don't block on flaky registry mirrors).
//   - *ErrImageNotFound on 404 from any registry, or 5xx from the local mirror
//     when localMode is true.
//
// A nil receiver is a no-op (returns nil), so callers can wire it
// unconditionally without a nil check.
func (p *ImagePreflighter) Preflight(ctx context.Context, image string) error {
	if p == nil || strings.TrimSpace(image) == "" {
		return nil
	}
	if p.cached(image) {
		return nil
	}

	host, repo, ref, ok := parseImageRef(image)
	if !ok {
		// No host (e.g. "library/postgres") or unparseable — fail open.
		// We don't want to block on docker.io shortnames or unusual refs.
		return nil
	}

	url := buildManifestURL(host, repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", manifestAcceptHeader)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	switch {
	case resp.StatusCode == http.StatusOK:
		p.markCached(image)
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return &ErrImageNotFound{Image: image, Reason: "registry returned 404"}
	case resp.StatusCode >= 500 && p.shouldTreat5xxAsMissing(host):
		return &ErrImageNotFound{
			Image:  image,
			Reason: fmt.Sprintf("local registry mirror returned %d (treated as missing)", resp.StatusCode),
		}
	default:
		return nil
	}
}

// PreflightWithBuildID is a convenience wrapper that injects the deploy's
// build_id into a returned ErrImageNotFound so the HTTP error body can
// surface it. Other failures pass through unchanged.
func (p *ImagePreflighter) PreflightWithBuildID(ctx context.Context, image, buildID string) error {
	err := p.Preflight(ctx, image)
	if nf, ok := AsImageNotFound(err); ok {
		nf.BuildID = buildID
		return nf
	}
	return err
}

func (p *ImagePreflighter) cached(image string) bool {
	v, ok := p.cache.Load(image)
	if !ok {
		return false
	}
	exp, _ := v.(time.Time)
	if time.Now().After(exp) {
		p.cache.Delete(image)
		return false
	}
	return true
}

func (p *ImagePreflighter) markCached(image string) {
	p.cache.Store(image, time.Now().Add(p.ttl))
}

func (p *ImagePreflighter) shouldTreat5xxAsMissing(host string) bool {
	if !p.localMode {
		return false
	}
	if len(p.localMirrorHosts) == 0 {
		return true
	}
	target := stripPort(host)
	for _, h := range p.localMirrorHosts {
		if strings.EqualFold(stripPort(h), target) {
			return true
		}
	}
	return false
}

// manifestAcceptHeader requests both OCI and Docker v2 manifest types.
// Some registries return 404 if no compatible Accept header is sent.
const manifestAcceptHeader = "application/vnd.oci.image.manifest.v1+json," +
	"application/vnd.oci.image.index.v1+json," +
	"application/vnd.docker.distribution.manifest.v2+json," +
	"application/vnd.docker.distribution.manifest.list.v2+json"

// parseImageRef splits an image reference into host, repo, and reference
// (tag or digest). Returns ok=false when the reference has no explicit host
// (Docker shortnames, "library/postgres", etc.) so we don't accidentally
// preflight against docker.io for every public image.
func parseImageRef(image string) (host, repo, ref string, ok bool) {
	work := strings.TrimSpace(image)
	if work == "" {
		return "", "", "", false
	}

	// Digest takes precedence over tag (image[@sha256:...]).
	if idx := strings.Index(work, "@"); idx > 0 {
		ref = work[idx+1:]
		work = work[:idx]
	} else if idx := strings.LastIndex(work, ":"); idx > 0 && !strings.Contains(work[idx+1:], "/") {
		ref = work[idx+1:]
		work = work[:idx]
	} else {
		ref = "latest"
	}

	parts := strings.SplitN(work, "/", 2)
	if len(parts) < 2 {
		return "", "", "", false
	}

	h := parts[0]
	if !looksLikeHost(h) {
		return "", "", "", false
	}
	return h, parts[1], ref, true
}

// looksLikeHost returns true for strings that are unambiguously a registry
// host (contain a dot, contain a port separator, or are localhost). This
// matches Docker's own host detection heuristic.
func looksLikeHost(s string) bool {
	if s == "" {
		return false
	}
	if s == "localhost" || strings.HasSuffix(s, ".localhost") {
		return true
	}
	return strings.Contains(s, ".") || strings.Contains(s, ":")
}

// buildManifestURL constructs the standard registry V2 manifest URL.
// Localhost-ish hosts default to http; everything else https — this matches
// the convention used by Docker, containerd, and the existing image
// resolver, and is what the local astro-registry mirror serves on.
func buildManifestURL(host, repo, ref string) string {
	scheme := "https"
	if isLikelyHTTPHost(host) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/v2/%s/manifests/%s", scheme, host, repo, ref)
}

func isLikelyHTTPHost(host string) bool {
	stripped := stripPort(host)
	return stripped == "localhost" ||
		strings.HasSuffix(stripped, ".localhost") ||
		strings.HasPrefix(stripped, "127.")
}

// stripPort removes a trailing :port from a host, leaving IPv6 addresses
// (which are bracketed) intact.
func stripPort(host string) string {
	i := strings.LastIndex(host, ":")
	if i < 0 {
		return host
	}
	if strings.Contains(host[i:], "]") {
		return host
	}
	return host[:i]
}
