// Package secretref resolves secret references in a dev environment through
// pluggable providers, and describes what each provider needs added to the
// containers that receive that environment.
package secretref

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
)

// Provider resolves the env values that reference one secret source.
type Provider interface {
	Name() string
	Matches(value string) bool
	Check(ctx context.Context) error
	Resolve(ctx context.Context, env map[string]string) (Resolution, error)
}

// Resolution is a provider's answer for the env entries it matched.
type Resolution struct {
	// Env replaces the matched entries' values.
	Env map[string]string
	// Runtime is nil for providers that only return values.
	Runtime *ContainerRuntime
	// Detail is shown in the start summary, e.g. "via daemon on :10144".
	Detail string
}

// ContainerRuntime is added to every container that receives the dev env.
type ContainerRuntime struct {
	Env        map[string]string
	Mounts     []Mount
	ExtraHosts []string
	// Proxy, when set, routes the container's outbound HTTPS through ProxyURL.
	// The compose layer composes NO_PROXY from NoProxy plus the project's own
	// service names, which only it knows.
	Proxy *Proxy
}

// Mount is a read-only bind mount.
type Mount struct {
	Source string
	Target string
}

// Proxy describes outbound routing for a container.
type Proxy struct {
	URL     string
	NoProxy []string
}

// Use reports one provider's share of a resolved env.
type Use struct {
	Provider string
	Keys     []string
	Detail   string
}

// Plan is the outcome of Resolve.
type Plan struct {
	Env     map[string]string
	Runtime *ContainerRuntime
	Uses    []Use
}

// CheckError carries every failed provider check, so start can report them all at once.
type CheckError struct {
	Failures map[string]error // provider name -> error
}

func (e *CheckError) Error() string {
	names := slices.Sorted(maps.Keys(e.Failures))
	msgs := make([]error, 0, len(names))
	for _, n := range names {
		msgs = append(msgs, fmt.Errorf("%s: %w", n, e.Failures[n]))
	}
	return errors.Join(msgs...).Error()
}

// Resolve finds the env values each provider matches, checks every provider in
// use, and returns the env with matched values replaced plus the merged
// container runtime. A value goes to the first provider that matches it. With
// no matches, Resolve returns env unchanged and a nil Runtime.
func Resolve(ctx context.Context, providers []Provider, env map[string]string) (*Plan, error) {
	matched := make([]map[string]string, len(providers))
	for _, k := range slices.Sorted(maps.Keys(env)) {
		for i, p := range providers {
			if p.Matches(env[k]) {
				if matched[i] == nil {
					matched[i] = map[string]string{}
				}
				matched[i][k] = env[k]
				break
			}
		}
	}

	failures := map[string]error{}
	for i, p := range providers {
		if matched[i] == nil {
			continue
		}
		if err := p.Check(ctx); err != nil {
			failures[p.Name()] = err
		}
	}
	if len(failures) > 0 {
		return nil, &CheckError{Failures: failures}
	}

	plan := &Plan{Env: maps.Clone(env)}
	if plan.Env == nil {
		plan.Env = map[string]string{}
	}
	for i, p := range providers {
		if matched[i] == nil {
			continue
		}
		res, err := p.Resolve(ctx, matched[i])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Name(), err)
		}
		maps.Copy(plan.Env, res.Env)
		plan.Runtime = mergeRuntime(plan.Runtime, res.Runtime)
		keys := slices.Collect(maps.Keys(matched[i]))
		sort.Strings(keys)
		plan.Uses = append(plan.Uses, Use{Provider: p.Name(), Keys: keys, Detail: res.Detail})
	}
	return plan, nil
}

func mergeRuntime(a, b *ContainerRuntime) *ContainerRuntime {
	if b == nil {
		return a
	}
	if a == nil {
		c := *b
		c.Env = maps.Clone(b.Env)
		return &c
	}
	if a.Env == nil {
		a.Env = map[string]string{}
	}
	maps.Copy(a.Env, b.Env)
	a.Mounts = append(a.Mounts, b.Mounts...)
	a.ExtraHosts = append(a.ExtraHosts, b.ExtraHosts...)
	if b.Proxy != nil {
		a.Proxy = b.Proxy
	}
	return a
}
