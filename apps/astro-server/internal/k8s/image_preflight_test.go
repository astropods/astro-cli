package k8s

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newPreflighterWithServer builds a preflighter wired to the given test
// server. It rewrites all requests so they hit srv regardless of the host
// embedded in the image — this lets us simulate "registry.localhost"
// without messing with the test process's DNS resolver.
func newPreflighterWithServer(t *testing.T, srv *httptest.Server, localMode bool) *ImagePreflighter {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 1 * time.Second}
			return d.DialContext(ctx, network, target.Host)
		},
	}
	p := NewImagePreflighter(localMode)
	p.SetClient(http.Client{Transport: transport, Timeout: 2 * time.Second})
	return p
}

// testImage is a host that resolves to the http (not https) scheme so the
// preflighter dials the httptest.Server (HTTP) without TLS handshake errors.
const testImage = "registry.localhost/acct/agent:b7396c13"

func TestImagePreflight_404ReturnsImageNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, false)
	err := p.Preflight(context.Background(), testImage)

	nf, ok := AsImageNotFound(err)
	if !ok {
		t.Fatalf("expected ErrImageNotFound, got %v", err)
	}
	if nf.Image != testImage {
		t.Errorf("Image=%q, want unchanged", nf.Image)
	}
	if !strings.Contains(nf.Reason, "404") {
		t.Errorf("Reason=%q, want it to mention 404", nf.Reason)
	}
}

func TestImagePreflight_LocalMirror5xxTreatedAsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, true)
	err := p.Preflight(context.Background(), "registry.localhost/acct/agent:b7396c13")

	nf, ok := AsImageNotFound(err)
	if !ok {
		t.Fatalf("expected ErrImageNotFound (local mirror 500-as-404 special case), got %v", err)
	}
	if !strings.Contains(nf.Reason, "500") {
		t.Errorf("Reason=%q, want it to mention 500", nf.Reason)
	}
}

func TestImagePreflight_NonLocalMode5xxFailsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, false)
	err := p.Preflight(context.Background(), testImage)
	if err != nil {
		t.Fatalf("expected nil (fail open on 5xx in non-local mode), got %v", err)
	}
}

func TestImagePreflight_200Proceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, false)
	err := p.Preflight(context.Background(), testImage)
	if err != nil {
		t.Fatalf("expected nil on 200, got %v", err)
	}
}

func TestImagePreflight_TransportErrorFailsOpen(t *testing.T) {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return nil, errors.New("simulated transport failure")
		},
	}
	p := NewImagePreflighter(false)
	p.SetClient(http.Client{Transport: transport, Timeout: 1 * time.Second})

	err := p.Preflight(context.Background(), testImage)
	if err != nil {
		t.Errorf("expected nil on transport error (fail open), got %v", err)
	}
}

func TestImagePreflight_PositiveResultCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, false)
	p.SetTTL(60 * time.Second)

	for i := 0; i < 5; i++ {
		if err := p.Preflight(context.Background(), testImage); err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 registry hit (rest cached), got %d", got)
	}
}

func TestImagePreflight_404NotCached(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, false)

	for i := 0; i < 3; i++ {
		err := p.Preflight(context.Background(), testImage)
		if _, ok := AsImageNotFound(err); !ok {
			t.Fatalf("iter %d: expected ErrImageNotFound, got %v", i, err)
		}
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 registry hits (404 must not cache), got %d", got)
	}
}

func TestImagePreflight_NilReceiverNoOp(t *testing.T) {
	var p *ImagePreflighter
	if err := p.Preflight(context.Background(), "registry.example.com/acct/agent:1"); err != nil {
		t.Fatalf("nil receiver should be a no-op, got %v", err)
	}
}

func TestImagePreflight_EmptyImageNoOp(t *testing.T) {
	p := NewImagePreflighter(false)
	if err := p.Preflight(context.Background(), ""); err != nil {
		t.Errorf("empty image should be a no-op, got %v", err)
	}
	if err := p.Preflight(context.Background(), "   "); err != nil {
		t.Errorf("whitespace image should be a no-op, got %v", err)
	}
}

func TestImagePreflight_PreflightWithBuildIDInjectsBuildID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, false)
	err := p.PreflightWithBuildID(context.Background(), testImage, "b7396c13")

	nf, ok := AsImageNotFound(err)
	if !ok {
		t.Fatalf("expected ErrImageNotFound, got %v", err)
	}
	if nf.BuildID != "b7396c13" {
		t.Errorf("BuildID=%q, want b7396c13", nf.BuildID)
	}
}

func TestImagePreflight_LocalMirrorScopedToConfiguredHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	p := newPreflighterWithServer(t, srv, true)
	p.localMirrorHosts = []string{"registry.localhost"}

	// localhost:5000 falls outside the configured allowlist — even though
	// 5xx came back, it must fail open (might be an unrelated registry).
	if err := p.Preflight(context.Background(), "localhost:5000/acct/agent:tag"); err != nil {
		t.Errorf("expected fail-open for non-mirror host, got %v", err)
	}
	if err := p.Preflight(context.Background(), "registry.localhost/acct/agent:tag"); err == nil {
		t.Errorf("expected ErrImageNotFound for mirror host, got nil")
	}
}

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		input        string
		wantOK       bool
		wantHost     string
		wantRepo     string
		wantRef      string
		shortNameTag string
	}{
		{"registry.example.com/acct/agent:tag", true, "registry.example.com", "acct/agent", "tag", ""},
		{"registry.example.com/acct/agent@sha256:abc", true, "registry.example.com", "acct/agent", "sha256:abc", ""},
		{"registry.localhost/acct/agent:b7396c13", true, "registry.localhost", "acct/agent", "b7396c13", ""},
		{"localhost:5000/acct/agent:tag", true, "localhost:5000", "acct/agent", "tag", ""},
		{"library/postgres", false, "", "", "", ""},
		{"postgres", false, "", "", "", ""},
		{"registry.example.com/acct/agent", true, "registry.example.com", "acct/agent", "latest", ""},
		{"123.dkr.ecr.us-east-1.amazonaws.com/ns/agent:abc", true, "123.dkr.ecr.us-east-1.amazonaws.com", "ns/agent", "abc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			h, r, ref, ok := parseImageRef(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v (h=%q r=%q ref=%q)", ok, tc.wantOK, h, r, ref)
			}
			if !ok {
				return
			}
			if h != tc.wantHost || r != tc.wantRepo || ref != tc.wantRef {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)", h, r, ref, tc.wantHost, tc.wantRepo, tc.wantRef)
			}
		})
	}
}

func TestBuildManifestURL_HTTPForLocalhost(t *testing.T) {
	cases := []struct {
		host, want string
	}{
		{"localhost:5000", "http://localhost:5000/v2/repo/manifests/tag"},
		{"registry.localhost", "http://registry.localhost/v2/repo/manifests/tag"},
		{"127.0.0.1:5000", "http://127.0.0.1:5000/v2/repo/manifests/tag"},
		{"registry.example.com", "https://registry.example.com/v2/repo/manifests/tag"},
		{"123.dkr.ecr.us-east-1.amazonaws.com", "https://123.dkr.ecr.us-east-1.amazonaws.com/v2/repo/manifests/tag"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := buildManifestURL(tc.host, "repo", "tag")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
