package compose

import (
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func proxyRuntime() *Runtime {
	return &Runtime{
		Env:        map[string]string{"HTTPS_PROXY": "http://host.docker.internal:10144", "NODE_USE_ENV_PROXY": "1"},
		Mounts:     []Mount{{Source: "/home/u/.ast/proxy/ca-bundle.pem", Target: "/etc/astro/proxy/ca-bundle.pem"}},
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
		NoProxy:    []string{"localhost", "127.0.0.1"},
	}
}

func TestApplyRuntime_NilRuntimeLeavesProjectUnchanged(t *testing.T) {
	p := runtimeTestProject()

	overridden := ApplyRuntime(p, nil)

	assert.Empty(t, overridden)
	assert.Equal(t, runtimeTestProject(), p)
}

func TestApplyRuntime_OnlyServicesThatReceiveTheDevEnv(t *testing.T) {
	p := runtimeTestProject()

	ApplyRuntime(p, proxyRuntime())

	for _, name := range []string{"agent", "ingestion-docs"} {
		svc := p.Services[name]
		assert.Equal(t, "http://host.docker.internal:10144", envVal(svc.Environment, "HTTPS_PROXY"), "%s routes through the proxy", name)
		require.Len(t, svc.Volumes, 1, "%s mounts the CA bundle", name)
		assert.Equal(t, types.ServiceVolumeConfig{Type: types.VolumeTypeBind, Source: "/home/u/.ast/proxy/ca-bundle.pem", Target: "/etc/astro/proxy/ca-bundle.pem", ReadOnly: true}, svc.Volumes[0])
		assert.Equal(t, []string{"host-gateway"}, svc.ExtraHosts["host.docker.internal"])
	}
	for _, name := range []string{"astro-messaging", "knowledge-vector"} {
		assert.Empty(t, envVal(p.Services[name].Environment, "HTTPS_PROXY"), "%s never receives the dev env, so it is not routed", name)
	}
}

func TestApplyRuntime_NoProxyCoversServicesAndGateway(t *testing.T) {
	p := runtimeTestProject()

	ApplyRuntime(p, proxyRuntime())

	noProxy := strings.Split(envVal(p.Services["agent"].Environment, "NO_PROXY"), ",")
	for _, host := range []string{"localhost", "127.0.0.1", "astro-messaging", "knowledge-vector", "ingestion-docs", "gateway.astropods.ai"} {
		assert.Contains(t, noProxy, host, "traffic to %s must not go through the proxy", host)
	}
	assert.Equal(t, envVal(p.Services["agent"].Environment, "NO_PROXY"), envVal(p.Services["agent"].Environment, "no_proxy"))
}

func TestApplyRuntime_NilNoProxyLeavesItUnset(t *testing.T) {
	p := runtimeTestProject()
	rt := proxyRuntime()
	rt.NoProxy = nil

	ApplyRuntime(p, rt)

	_, set := p.Services["agent"].Environment["NO_PROXY"]
	assert.False(t, set)
}

func TestApplyRuntime_ExistingKeysWinAndAreReported(t *testing.T) {
	p := runtimeTestProject()
	agent := p.Services["agent"]
	agent.Environment["HTTPS_PROXY"] = strp("http://corp-proxy:3128")
	p.Services["agent"] = agent

	overridden := ApplyRuntime(p, proxyRuntime())

	assert.Equal(t, []string{"HTTPS_PROXY"}, overridden)
	assert.Equal(t, "http://corp-proxy:3128", envVal(p.Services["agent"].Environment, "HTTPS_PROXY"), "a value the agent already sets is kept")
}
