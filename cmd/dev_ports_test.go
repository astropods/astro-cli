package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	composeBuilder "github.com/astropods/astro-cli/internal/compose"
)

func devPortsProject() *types.Project {
	return &types.Project{Services: types.Services{
		"astro-messaging": {Name: "astro-messaging", Ports: []types.ServicePortConfig{
			{Target: 9090, Published: "19090"},
			{Target: 8080, Published: "3110"},
		}},
		"agent":  {Name: "agent", Ports: []types.ServicePortConfig{{Target: 8080, Published: "3120"}}},
		"worker": {Name: "worker"},
	}}
}

func TestReleaseBusyHostPorts(t *testing.T) {
	tests := []struct {
		name      string
		busy      map[string]bool
		wantMoved []string
		want      map[string][]string // service to published host ports, in order
	}{
		{
			name: "nothing busy keeps every port",
			busy: map[string]bool{},
			want: map[string][]string{"astro-messaging": {"19090", "3110"}, "agent": {"3120"}},
		},
		{
			name:      "a busy port is left for Docker to assign",
			busy:      map[string]bool{"3110": true},
			wantMoved: []string{"astro-messaging:3110"},
			want:      map[string][]string{"astro-messaging": {"19090", ""}, "agent": {"3120"}},
		},
		{
			name:      "every busy port across services",
			busy:      map[string]bool{"19090": true, "3110": true, "3120": true},
			wantMoved: []string{"agent:3120", "astro-messaging:19090", "astro-messaging:3110"},
			want:      map[string][]string{"astro-messaging": {"", ""}, "agent": {""}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := devPortsProject()
			original := p.Services["astro-messaging"].Ports

			moved := releaseBusyHostPorts(p, func(port string) bool { return tt.busy[port] })

			assert.Equal(t, tt.wantMoved, moved)
			for service, want := range tt.want {
				var got []string
				for _, port := range p.Services[service].Ports {
					got = append(got, port.Published)
				}
				assert.Equal(t, want, got, service)
			}
			assert.Empty(t, p.Services["worker"].Ports)
			assert.Equal(t, "3110", original[1].Published, "the input slice must not be modified in place")
		})
	}
}

func TestHostPortBusy(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)

	assert.True(t, hostPortBusy(port))
	require.NoError(t, l.Close())
	assert.False(t, hostPortBusy(port))
}

func TestHostPortsFromContainers(t *testing.T) {
	items := []container.Summary{
		{
			Labels: map[string]string{api.ServiceLabel: "astro-messaging"},
			Ports: []container.PortSummary{
				{IP: netip.MustParseAddr("::"), PrivatePort: 8080, PublicPort: 55001},
				{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 8080, PublicPort: 55001},
				{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 9090, PublicPort: 19090},
			},
		},
		{
			Labels: map[string]string{api.ServiceLabel: "agent"},
			Ports:  []container.PortSummary{{PrivatePort: 80}}, // exposed, not published
		},
	}

	ports := hostPortsFromContainers(items)

	assert.Equal(t, devHostPorts{"astro-messaging": {8080: 55001, 9090: 19090}}, ports)
	assert.Equal(t, "55001", ports.host("astro-messaging", 8080))
	assert.Empty(t, ports.host("agent", 80), "an unpublished port must not fall back to a well-known one")
}

func TestMessagingURLFromCmdline(t *testing.T) {
	tests := []struct {
		name    string
		cmdline string
		want    string
	}{
		{"separate value", "/bin/ast chatui-serve --addr 127.0.0.1:3100 --messaging-url http://127.0.0.1:3110 --agent-name a", "http://127.0.0.1:3110"},
		{"equals form", "ast chatui-serve --messaging-url=http://127.0.0.1:55001", "http://127.0.0.1:55001"},
		{"missing", "ast chatui-serve --addr 127.0.0.1:3100", ""},
		{"flag without value", "ast chatui-serve --messaging-url", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, messagingURLFromCmdline(tt.cmdline))
		})
	}
}

func TestSidecarAnswers(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + l.Addr().String()

	assert.True(t, sidecarAnswers(url))
	require.NoError(t, l.Close())
	assert.False(t, sidecarAnswers(url))
	assert.False(t, sidecarAnswers(""))
}

func TestChatURLForAddr(t *testing.T) {
	assert.Equal(t, "http://localhost:3100", chatURLForAddr("127.0.0.1:3100"))
	assert.Equal(t, "http://localhost:55123", chatURLForAddr("127.0.0.1:55123"))
}

func TestReadyURLsFor(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "ag",
		Agent: spec.Container{Image: "x", Interfaces: &spec.Interfaces{Frontend: true}},
		Ingestion: map[string]spec.Ingestion{
			"hooks":   {Trigger: spec.IngestionTrigger{Type: "webhook"}},
			"nightly": {Trigger: spec.IngestionTrigger{Type: "schedule"}},
		},
	}
	ports := devHostPorts{"agent": {80: 55002}}

	urls := readyURLsFor(s, ports, "127.0.0.1:55003")

	assert.Equal(t, readyURLs{
		Frontend: "http://localhost:55002",
		Chat:     "http://localhost:55003",
		Webhooks: map[string]string{},
	}, urls, "the webhook publishes no port, so it gets no URL")
}

func TestLabelDevChatURL(t *testing.T) {
	p := devPortsProject()
	shared := types.Labels{api.ProjectLabel: "ag"}
	for name, svc := range p.Services {
		svc.CustomLabels = shared
		p.Services[name] = svc
	}

	labelDevChatURL(p, "http://localhost:55003")

	for name, svc := range p.Services {
		assert.Equal(t, "http://localhost:55003", svc.CustomLabels[composeBuilder.DevChatURLLabel], name)
		assert.Equal(t, "ag", svc.CustomLabels[api.ProjectLabel], name)
	}
	assert.NotContains(t, shared, composeBuilder.DevChatURLLabel, "the original label map must not be modified")
}

func TestMsgDevPortsMovedIsSilentWhenNothingMoved(t *testing.T) {
	assert.Empty(t, msgDevPortsMoved(nil))
}

func TestIsPortAllocatedErr(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("Bind for 0.0.0.0:3110 failed: port is already allocated"), true},
		{errors.New("listen tcp4 0.0.0.0:3110: bind: Address already in use"), true},
		{errors.New("image not found"), false},
		{nil, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isPortAllocatedErr(tt.err), fmt.Sprint(tt.err))
	}
}
