package compose

import (
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/secretref"
)

func strp(s string) *string { return &s }

func runtimeTestProject() *types.Project {
	return &types.Project{Services: types.Services{
		"agent":            {Name: "agent", Environment: types.MappingWithEquals{"ASTRO_GATEWAY_URL": strp("https://gateway.astropods.ai/v1")}},
		"ingestion-docs":   {Name: "ingestion-docs"},
		"astro-messaging":  {Name: "astro-messaging"},
		"knowledge-vector": {Name: "knowledge-vector"},
	}}
}

func passportLikeRuntime() *secretref.ContainerRuntime {
	return &secretref.ContainerRuntime{
		Env:        map[string]string{"NODE_USE_ENV_PROXY": "1"},
		Mounts:     []secretref.Mount{{Source: "/home/u/.ast/passport/ca-bundle.pem", Target: "/etc/astro/passport/ca-bundle.pem"}},
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
		Proxy:      &secretref.Proxy{URL: "http://host.docker.internal:10144", NoProxy: []string{"localhost", "127.0.0.1"}},
	}
}

func TestApplyRuntime_NilRuntimeLeavesProjectUnchanged(t *testing.T) {
	p := runtimeTestProject()
	before := runtimeTestProject()

	overridden := ApplyRuntime(p, nil)

	assert.Empty(t, overridden)
	assert.Equal(t, before, p)
}

func TestApplyRuntime_OnlyServicesThatReceiveTheDevEnv(t *testing.T) {
	p := runtimeTestProject()

	ApplyRuntime(p, passportLikeRuntime())

	for _, name := range []string{"agent", "ingestion-docs"} {
		svc := p.Services[name]
		assert.Equal(t, "http://host.docker.internal:10144", envVal(svc.Environment, "HTTPS_PROXY"), "%s routes through the proxy", name)
		assert.Equal(t, "http://host.docker.internal:10144", envVal(svc.Environment, "https_proxy"), "%s gets the lowercase form too", name)
		assert.Equal(t, "1", envVal(svc.Environment, "NODE_USE_ENV_PROXY"))
		require.Len(t, svc.Volumes, 1, "%s mounts the CA bundle", name)
		assert.Equal(t, types.ServiceVolumeConfig{Type: types.VolumeTypeBind, Source: "/home/u/.ast/passport/ca-bundle.pem", Target: "/etc/astro/passport/ca-bundle.pem", ReadOnly: true}, svc.Volumes[0])
		assert.Equal(t, []string{"host-gateway"}, svc.ExtraHosts["host.docker.internal"])
	}
	for _, name := range []string{"astro-messaging", "knowledge-vector"} {
		svc := p.Services[name]
		assert.Empty(t, envVal(svc.Environment, "HTTPS_PROXY"), "%s never receives the dev env, so it is not routed", name)
		assert.Empty(t, svc.Volumes)
	}
}

func TestApplyRuntime_NoProxyCoversServicesAndGateway(t *testing.T) {
	p := runtimeTestProject()

	ApplyRuntime(p, passportLikeRuntime())

	noProxy := strings.Split(envVal(p.Services["agent"].Environment, "NO_PROXY"), ",")
	for _, host := range []string{"localhost", "127.0.0.1", "astro-messaging", "knowledge-vector", "ingestion-docs", "gateway.astropods.ai"} {
		assert.Contains(t, noProxy, host, "traffic to %s must not go through the proxy", host)
	}
	assert.Equal(t, envVal(p.Services["agent"].Environment, "NO_PROXY"), envVal(p.Services["agent"].Environment, "no_proxy"))
	assert.NotContains(t, strings.Split(envVal(p.Services["ingestion-docs"].Environment, "NO_PROXY"), ","), "gateway.astropods.ai",
		"the gateway host comes from each service's own ASTRO_GATEWAY_URL")
}

func TestApplyRuntime_ExistingKeysWinAndAreReported(t *testing.T) {
	p := runtimeTestProject()
	agent := p.Services["agent"]
	agent.Environment["HTTPS_PROXY"] = strp("http://corp-proxy:3128")
	p.Services["agent"] = agent

	overridden := ApplyRuntime(p, passportLikeRuntime())

	assert.Equal(t, []string{"HTTPS_PROXY"}, overridden)
	assert.Equal(t, "http://corp-proxy:3128", envVal(p.Services["agent"].Environment, "HTTPS_PROXY"), "a value the agent already sets is kept")
	assert.Equal(t, "http://host.docker.internal:10144", envVal(p.Services["ingestion-docs"].Environment, "HTTPS_PROXY"))
}

func TestApplyRuntime_ExtraHostsNotDuplicated(t *testing.T) {
	p := runtimeTestProject()
	rt := passportLikeRuntime()
	rt.ExtraHosts = append(rt.ExtraHosts, "host.docker.internal:host-gateway")

	ApplyRuntime(p, rt)

	assert.Equal(t, []string{"host-gateway"}, p.Services["agent"].ExtraHosts["host.docker.internal"])
}
