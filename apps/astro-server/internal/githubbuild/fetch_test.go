package githubbuild

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// withTestHTTPClient temporarily replaces the package-level httpClient with
// one that redirects all requests to srv, then restores the original on cleanup.
func withTestHTTPClient(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := httpClient
	t.Cleanup(func() { httpClient = orig })
	u, _ := url.Parse(srv.URL)
	httpClient = &http.Client{
		Transport: &rewriteHostTransport{target: u},
	}
}

type rewriteHostTransport struct {
	target *url.URL
}

func (rt *rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = rt.target.Host
	r2.URL.Scheme = rt.target.Scheme
	return http.DefaultTransport.RoundTrip(r2)
}

func TestFetchFileContent_Success(t *testing.T) {
	const content = "spec: package/v1\nname: my-agent\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, content)
	}))
	defer srv.Close()
	withTestHTTPClient(t, srv)

	got, err := FetchFileContent(context.Background(), "mytoken", "owner/repo", "abc123", "astropods.yml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestFetchFileContent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	withTestHTTPClient(t, srv)

	got, err := FetchFileContent(context.Background(), "mytoken", "owner/repo", "abc123", "astropods.yml")
	if err != nil {
		t.Fatalf("expected nil error for 404, got: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing file, got %q", got)
	}
}

func TestFetchFileContent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()
	withTestHTTPClient(t, srv)

	_, err := FetchFileContent(context.Background(), "mytoken", "owner/repo", "abc123", "astropods.yml")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchAstroSpec_ValidYAML(t *testing.T) {
	yaml := "spec: package/v1\nname: my-agent\nagent:\n  image: myimage\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, yaml)
	}))
	defer srv.Close()
	withTestHTTPClient(t, srv)

	s, raw, err := FetchAstroSpec(context.Background(), "tok", "owner/repo", "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "my-agent" {
		t.Errorf("Name = %q, want %q", s.Name, "my-agent")
	}
	if raw != yaml {
		t.Errorf("raw YAML mismatch: got %q, want %q", raw, yaml)
	}
}

func TestFetchAstroSpec_InvalidYAML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, ":\t invalid: yaml: [")
	}))
	defer srv.Close()
	withTestHTTPClient(t, srv)

	_, _, err := FetchAstroSpec(context.Background(), "tok", "owner/repo", "abc")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parse astropods.yml") {
		t.Errorf("error should mention parse astropods.yml, got: %v", err)
	}
}

func TestCollectComponentBuilds_AgentOnly(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Build: &spec.BuildConfig{Context: ".", Dockerfile: "Dockerfile"},
		},
	}
	builds := CollectComponentBuilds(s, "my-agent")
	if len(builds) != 1 {
		t.Fatalf("got %d builds, want 1", len(builds))
	}
	if builds[0].Suffix != "agent" {
		t.Errorf("Suffix = %q, want %q", builds[0].Suffix, "agent")
	}
	if builds[0].Name != "my-agent" {
		t.Errorf("Name = %q, want %q", builds[0].Name, "my-agent")
	}
}

func TestCollectComponentBuilds_WithSidecar(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Build: &spec.BuildConfig{Context: "."},
		},
		Integrations: map[string]spec.Integration{
			"search": {
				Container: &spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: "./tools/search"},
				},
			},
		},
	}
	builds := CollectComponentBuilds(s, "my-agent")
	if len(builds) != 2 {
		t.Fatalf("got %d builds, want 2", len(builds))
	}
	var foundTool bool
	for _, b := range builds {
		if b.Suffix == "tool-search" {
			foundTool = true
			if b.Name != "my-agent-tool-search" {
				t.Errorf("tool Name = %q, want %q", b.Name, "my-agent-tool-search")
			}
		}
	}
	if !foundTool {
		t.Error("expected tool-search build, not found")
	}
}

func TestCollectComponentBuilds_SkipsNoBuildBlock(t *testing.T) {
	// A tool without a build block should not produce a ComponentBuild.
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{},
		Integrations: map[string]spec.Integration{
			"cloud-search": {Provider: "tavily"},
		},
	}
	builds := CollectComponentBuilds(s, "my-agent")
	// Only agent entry (with empty build config) — no tool build.
	for _, b := range builds {
		if b.Suffix == "tool-cloud-search" {
			t.Error("did not expect a build for a provider-mode tool")
		}
	}
}
