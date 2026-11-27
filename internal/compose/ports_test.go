package compose

import (
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildProject_AgentPorts(t *testing.T) {
	devPort := &spec.Dev{Interfaces: &spec.DevInterfaces{Frontend: &spec.DevFrontend{Port: 5173}}}
	tests := []struct {
		name      string
		agent     spec.Container
		dev       *spec.Dev
		wantPorts map[uint32]string
	}{
		{
			name: "frontend port on 3200 and internal port on its own number",
			agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
				Ports: map[string]spec.AgentPort{
					"web":   {Port: 3000, Expose: true},
					"admin": {Port: 9000},
				},
			},
			wantPorts: map[uint32]string{3000: FrontendHostPort, 9000: "9000"},
		},
		{
			name: "open port without frontend publishes on its own number",
			agent: spec.Container{
				Image: "agent:latest",
				Ports: map[string]spec.AgentPort{"hooks": {Port: 3000, Expose: true}},
			},
			wantPorts: map[uint32]string{3000: "3000"},
		},
		{
			name: "deprecated dev port is ignored when agent.ports is set",
			agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
				Ports:      map[string]spec.AgentPort{"web": {Port: 3000, Expose: true}},
			},
			dev:       devPort,
			wantPorts: map[uint32]string{3000: FrontendHostPort},
		},
		{
			name: "deprecated dev port replaces the default 80 without agent.ports",
			agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
			},
			dev:       devPort,
			wantPorts: map[uint32]string{5173: FrontendHostPort},
		},
		{
			name:      "no ports and no frontend publishes nothing",
			agent:     spec.Container{Image: "agent:latest"},
			wantPorts: map[uint32]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &spec.AstroSpec{Name: "my-agent", Agent: tt.agent, Dev: tt.dev}
			project, err := BuildProject(s, "/work", nil)
			require.NoError(t, err)
			got := map[uint32]string{}
			for _, p := range project.Services["agent"].Ports {
				got[p.Target] = p.Published
			}
			assert.Equal(t, tt.wantPorts, got, "agent container port to published host port")
		})
	}
}

func TestBuildEnvironment_AgentPortEnv(t *testing.T) {
	tests := []struct {
		name   string
		agent  spec.Container
		dev    *spec.Dev
		want   map[string]string
		absent []string
	}{
		{
			name: "frontend port and internal port",
			agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
				Ports: map[string]spec.AgentPort{
					"web":   {Port: 3000, Expose: true},
					"admin": {Port: 9000},
				},
			},
			want: map[string]string{"PORT": "3000", "ASTRO_PORT_WEB": "3000", "ASTRO_PORT_ADMIN": "9000"},
		},
		{
			name: "internal port only sets no PORT",
			agent: spec.Container{
				Image: "agent:latest",
				Ports: map[string]spec.AgentPort{"admin": {Port: 9000}},
			},
			want:   map[string]string{"ASTRO_PORT_ADMIN": "9000"},
			absent: []string{"PORT"},
		},
		{
			name: "deprecated dev port sets PORT and ASTRO_PORT_HTTP to match the published target",
			agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
			},
			dev:  &spec.Dev{Interfaces: &spec.DevInterfaces{Frontend: &spec.DevFrontend{Port: 5173}}},
			want: map[string]string{"PORT": "5173", "ASTRO_PORT_HTTP": "5173"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := BuildEnvironment(&spec.AstroSpec{Name: "my-agent", Agent: tt.agent, Dev: tt.dev}, nil)
			for k, v := range tt.want {
				assert.Equal(t, v, envVal(env, k), "env %s", k)
			}
			for _, k := range tt.absent {
				assert.NotContains(t, env, k, "env %s must be unset", k)
			}
		})
	}
}

func TestFrontendTargetPort(t *testing.T) {
	assert.Equal(t, uint32(0), FrontendTargetPort(&spec.AstroSpec{Agent: spec.Container{Image: "x"}}), "an agent without a frontend has no frontend target port")
	assert.Equal(t, uint32(80), FrontendTargetPort(&spec.AstroSpec{Agent: spec.Container{Image: "x", Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}}}}), "frontend true alone targets the default 80")
	assert.Equal(t, uint32(3000), FrontendTargetPort(&spec.AstroSpec{Agent: spec.Container{
		Image:      "x",
		Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true, Port: "web"}},
		Ports:      map[string]spec.AgentPort{"web": {Port: 3000, Expose: true}},
	}}), "a named frontend targets its declared port")
}

func TestBuildProject_AgentPortClashingWithAProjectHostPortGetsAFreeOne(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
			Ports: map[string]spec.AgentPort{
				"web":   {Port: 3000, Expose: true},
				"admin": {Port: 3200},
				"hooks": {Port: 3001},
				"api":   {Port: 9000},
			},
		},
		Ingestion: map[string]spec.Ingestion{
			"inbox": {Container: spec.ContainerConfig{Image: "ingest:latest"}, Trigger: spec.IngestionTrigger{Type: "webhook"}},
		},
	}
	project, err := BuildProject(s, "/work", nil)
	require.NoError(t, err)
	got := map[uint32]string{}
	for _, p := range project.Services["agent"].Ports {
		got[p.Target] = p.Published
	}
	assert.Equal(t, map[uint32]string{3000: FrontendHostPort, 3200: "", 3001: "", 9000: "9000"}, got,
		"a port whose number the frontend or the webhook ingestion already publishes is left for Docker to place")
}

func TestBuildProject_IngestionGetsNoAgentPortEnv(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Frontend: spec.Frontend{Enabled: true}},
			Ports:      map[string]spec.AgentPort{"web": {Port: 3000, Expose: true}, "admin": {Port: 9000}},
		},
		Ingestion: map[string]spec.Ingestion{
			"inbox": {Container: spec.ContainerConfig{Image: "ingest:latest"}, Trigger: spec.IngestionTrigger{Type: "webhook"}},
		},
	}
	project, err := BuildProject(s, "/work", nil)
	require.NoError(t, err)
	ingestionEnv := project.Services["ingestion-inbox"].Environment
	for _, k := range []string{"PORT", "ASTRO_PORT_WEB", "ASTRO_PORT_ADMIN"} {
		assert.NotContains(t, ingestionEnv, k, "the webhook ingestion must not inherit the agent's %s", k)
	}
	assert.Equal(t, "3000", envVal(project.Services["agent"].Environment, "PORT"), "the agent still gets its frontend PORT")
}
