package compose

import (
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"

	"github.com/astropods/astro-cli/internal/secretref"
)

// ApplyRuntime adds rt to every service that receives the dev env (the agent
// and each ingestion job). A key the service already sets keeps its value; the
// returned keys are those overrides, so the caller can warn.
func ApplyRuntime(p *types.Project, rt *secretref.ContainerRuntime) []string {
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
		if rt.Proxy != nil {
			noProxy := strings.Join(noProxyFor(p, svc, rt.Proxy.NoProxy), ",")
			for _, k := range []string{"HTTPS_PROXY", "https_proxy"} {
				env[k] = rt.Proxy.URL
			}
			for _, k := range []string{"NO_PROXY", "no_proxy"} {
				env[k] = noProxy
			}
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
