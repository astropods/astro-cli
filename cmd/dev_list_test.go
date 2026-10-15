package cmd

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/chatui"
	composeBuilder "github.com/astropods/astro-cli/internal/compose"
)

func TestLocalAgentsFromContainers(t *testing.T) {
	labeled := func(service string) map[string]string {
		return map[string]string{
			api.ProjectLabel:             "poweruser",
			api.ServiceLabel:             service,
			api.WorkingDirLabel:          "/home/me/poweruser",
			composeBuilder.DevAgentLabel: "@acme/poweruser",
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
		{Name: "@acme/poweruser", Dir: "/home/me/poweruser", Containers: 2},
		{Name: "capcom", Dir: "/home/me/capcom", Containers: 2},
	}, localAgentsFromContainers(items))
}

func TestFindChatUIs(t *testing.T) {
	health := func(body string) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc(chatui.HealthPath, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		})
		return httptest.NewServer(mux)
	}
	port := func(s *httptest.Server) int { return s.Listener.Addr().(*net.TCPAddr).Port }

	current := health(`{"ok":true,"pid":1,"agent":"@acme/poweruser"}`)
	defer current.Close()
	older := health(`{"ok":true,"pid":2}`) // a worker from a CLI that predates the agent field
	defer older.Close()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedPort := closed.Addr().(*net.TCPAddr).Port
	require.NoError(t, closed.Close())

	found := findChatUIs([]int{port(current), port(older), closedPort})

	assert.Equal(t, map[string]string{"@acme/poweruser": chatURLForAddr(current.Listener.Addr().String())}, found)
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
