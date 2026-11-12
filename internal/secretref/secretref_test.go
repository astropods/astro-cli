package secretref

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	name     string
	prefix   string
	checkErr error
	runtime  *ContainerRuntime
	checked  int
	resolved map[string]string
}

func (f *fakeProvider) Name() string              { return f.name }
func (f *fakeProvider) Matches(value string) bool { return strings.HasPrefix(value, f.prefix) }
func (f *fakeProvider) Check(context.Context) error {
	f.checked++
	return f.checkErr
}
func (f *fakeProvider) Resolve(_ context.Context, env map[string]string) (Resolution, error) {
	f.resolved = env
	out := map[string]string{}
	for k, v := range env {
		out[k] = "resolved:" + strings.TrimPrefix(v, f.prefix)
	}
	return Resolution{Env: out, Runtime: f.runtime, Detail: "detail"}, nil
}

func TestResolve_NoReferences(t *testing.T) {
	p := &fakeProvider{name: "A", prefix: "a://"}
	env := map[string]string{"PLAIN": "value"}

	plan, err := Resolve(context.Background(), []Provider{p}, env)

	require.NoError(t, err)
	assert.Equal(t, env, plan.Env, "env without references is returned unchanged")
	assert.Nil(t, plan.Runtime, "no provider in use means no container runtime")
	assert.Empty(t, plan.Uses)
	assert.Zero(t, p.checked, "a provider with no matching values is never checked")
}

func TestResolve_ReplacesMatchedValuesAndMergesRuntime(t *testing.T) {
	a := &fakeProvider{name: "A", prefix: "a://", runtime: &ContainerRuntime{Env: map[string]string{"FROM_A": "1"}, ExtraHosts: []string{"h:1"}}}
	b := &fakeProvider{name: "B", prefix: "b://", runtime: &ContainerRuntime{Env: map[string]string{"FROM_B": "1"}, Proxy: &Proxy{URL: "http://p"}}}
	env := map[string]string{"KEY_A": "a://one", "KEY_B": "b://two", "PLAIN": "value"}

	plan, err := Resolve(context.Background(), []Provider{a, b}, env)

	require.NoError(t, err)
	assert.Equal(t, map[string]string{"KEY_A": "resolved:one", "KEY_B": "resolved:two", "PLAIN": "value"}, plan.Env)
	assert.Equal(t, map[string]string{"KEY_A": "a://one"}, a.resolved, "each provider resolves only the values it matched")
	assert.Equal(t, map[string]string{"FROM_A": "1", "FROM_B": "1"}, plan.Runtime.Env)
	assert.Equal(t, []string{"h:1"}, plan.Runtime.ExtraHosts)
	require.NotNil(t, plan.Runtime.Proxy)
	assert.Equal(t, "http://p", plan.Runtime.Proxy.URL)
	assert.Equal(t, []Use{{Provider: "A", Keys: []string{"KEY_A"}, Detail: "detail"}, {Provider: "B", Keys: []string{"KEY_B"}, Detail: "detail"}}, plan.Uses)
	assert.Equal(t, "a://one", env["KEY_A"], "the caller's env map is not mutated")
}

func TestResolve_FirstMatchingProviderWins(t *testing.T) {
	first := &fakeProvider{name: "First", prefix: "x"}
	second := &fakeProvider{name: "Second", prefix: "x"}

	plan, err := Resolve(context.Background(), []Provider{first, second}, map[string]string{"K": "x1"})

	require.NoError(t, err)
	assert.Equal(t, []Use{{Provider: "First", Keys: []string{"K"}, Detail: "detail"}}, plan.Uses)
	assert.Zero(t, second.checked, "a provider that matched nothing is not checked")
}

func TestResolve_ReportsEveryFailedCheckBeforeResolving(t *testing.T) {
	a := &fakeProvider{name: "A", prefix: "a://", checkErr: errors.New("a down")}
	b := &fakeProvider{name: "B", prefix: "b://", checkErr: errors.New("b down")}

	_, err := Resolve(context.Background(), []Provider{a, b}, map[string]string{"KA": "a://1", "KB": "b://2"})

	var checkErr *CheckError
	require.ErrorAs(t, err, &checkErr)
	assert.Len(t, checkErr.Failures, 2, "both failures are reported, not just the first")
	assert.Nil(t, a.resolved, "nothing resolves when any check fails")
	assert.Nil(t, b.resolved)
}
