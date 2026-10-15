package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"

	"github.com/astropods/astro-cli/internal/chatui"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
)

func TestLocalAgentsFromContainers(t *testing.T) {
	labeled := func(service string) map[string]string {
		return map[string]string{
			api.ProjectLabel:               "poweruser",
			api.ServiceLabel:               service,
			api.WorkingDirLabel:            "/home/me/poweruser",
			composeBuilder.DevAgentLabel:   "@acme/poweruser",
			composeBuilder.DevChatURLLabel: "http://localhost:3100",
		}
	}
	items := []container.Summary{
		{Labels: labeled("agent")},
		{Labels: labeled("astro-messaging")},
		// Started by a CLI that predates the dev labels: found by its sidecar only.
		{Labels: map[string]string{
			api.ProjectLabel:    "capcom",
			api.ServiceLabel:    "astro-messaging",
			api.WorkingDirLabel: "/home/me/capcom",
		}},
		{Labels: map[string]string{api.ProjectLabel: "capcom", api.ServiceLabel: "agent"}},
		{Labels: map[string]string{api.ProjectLabel: "postgres-tools", api.ServiceLabel: "db"}}, // not a dev project
		{Labels: map[string]string{api.ServiceLabel: "astro-messaging"}},                        // not a compose project
	}

	assert.Equal(t, []localAgent{
		{Name: "@acme/poweruser", Dir: "/home/me/poweruser", ChatURL: "http://localhost:3100", Containers: 2},
		{Name: "capcom", Dir: "/home/me/capcom", Containers: 2},
	}, localAgentsFromContainers(items))
}

func TestChatUIServing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(chatui.HealthPath, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := httptest.NewServer(mux)
	c := &http.Client{Timeout: time.Second}

	assert.True(t, chatUIServing(c, srv.URL))
	srv.Close()
	assert.False(t, chatUIServing(c, srv.URL))
}

func TestIsLocalHTTPURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://localhost:3100", true},
		{"http://127.0.0.1:55001", true},
		{"http://169.254.169.254/latest", false},
		{"https://localhost:3100", false},
		{"http://localhost.example.com:3100", false},
		{"::not a url", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isLocalHTTPURL(tt.url), tt.url)
	}
}

func TestShortenHome(t *testing.T) {
	tests := []struct{ path, home, want string }{
		{"/home/me/agents/a", "/home/me", "~/agents/a"},
		{"/srv/agents/a", "/home/me", "/srv/agents/a"},
		{"/home/me/a", "", "/home/me/a"},
		{"", "/home/me", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, shortenHome(tt.path, tt.home), tt.path)
	}
}
