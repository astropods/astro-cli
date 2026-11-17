package compose

import (
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
)

// Runtime is added to every container that receives the dev env.
type Runtime struct {
	Env        map[string]string
	Mounts     []Mount
	ExtraHosts []string
	// NoProxy, when set, becomes NO_PROXY and no_proxy, plus the project's own
	// service names and the AI Gateway host.
	NoProxy []string
}

// Mount is a read-only bind mount.
type Mount struct {
	Source string
	Target string
}

// ApplyRuntime adds rt to the agent and each ingestion job. A key the service
// already sets keeps its value; the returned keys are those overrides.
func ApplyRuntime(p *types.Project, rt *Runtime) []string {
	if rt == nil {
		return nil
	}
	overridden := map[string]bool{}
	for name, svc := range p.Services {
		if !receivesDevEnv(name) {
			continue
		}
		if svc.Environment == nil {
			svc.Environment = types.MappingWithEquals{}
		}
		env := maps.Clone(rt.Env)
		if env == nil {
			env = map[string]string{}
		}
		if rt.NoProxy != nil {
			noProxy := strings.Join(noProxyFor(p, svc, rt.NoProxy), ",")
			env["NO_PROXY"] = noProxy
			env["no_proxy"] = noProxy
		}
		for k, v := range env {
			if _, set := svc.Environment[k]; set {
				overridden[k] = true
				continue
			}
			svc.Environment[k] = &v
		}
		for _, m := range rt.Mounts {
			svc.Volumes = append(svc.Volumes, types.ServiceVolumeConfig{
				Type: types.VolumeTypeBind, Source: m.Source, Target: m.Target, ReadOnly: true,
			})
		}
		if len(rt.ExtraHosts) > 0 {
			if svc.ExtraHosts == nil {
				svc.ExtraHosts = types.HostsList{}
			}
			for _, h := range rt.ExtraHosts {
				host, ip, _ := strings.Cut(h, ":")
				if !slices.Contains(svc.ExtraHosts[host], ip) {
					svc.ExtraHosts[host] = append(svc.ExtraHosts[host], ip)
				}
			}
		}
		p.Services[name] = svc
	}
	return slices.Sorted(maps.Keys(overridden))
}

func receivesDevEnv(service string) bool {
	return service == "agent" || strings.HasPrefix(service, "ingestion-")
}

func noProxyFor(p *types.Project, svc types.ServiceConfig, base []string) []string {
	hosts := slices.Clone(base)
	hosts = append(hosts, slices.Sorted(maps.Keys(p.Services))...)
	if v := svc.Environment["ASTRO_GATEWAY_URL"]; v != nil {
		if u, err := url.Parse(*v); err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}
