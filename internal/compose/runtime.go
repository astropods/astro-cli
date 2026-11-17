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
// already sets keeps its value, and so does the rest of its group (see
// envGroup); the returned keys are the service's own keys that caused a skip.
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
		setBy := map[string][]string{}
		for k := range svc.Environment {
			g := envGroup(k)
			setBy[g] = append(setBy[g], k)
		}
		for k, v := range env {
			if keys, set := setBy[envGroup(k)]; set {
				for _, sk := range keys {
					overridden[sk] = true
				}
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

// envGroup names the keys that must be set together: both case forms of a
// proxy variable, and the three CA variables, so a service never ends up with
// half its own setting and half the host's.
func envGroup(key string) string {
	switch upper := strings.ToUpper(key); upper {
	case "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY":
		return upper
	case "SSL_CERT_FILE", "NODE_EXTRA_CA_CERTS", "REQUESTS_CA_BUNDLE":
		return "CA_FILES"
	}
	return key
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
