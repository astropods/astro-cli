package deployment

import (
	"reflect"
	"sort"
	"testing"
)

func TestProject_SplitsBySecret(t *testing.T) {
	rows := []Resolution{
		{Role: RoleAgent, EnvName: "ASTRO_AGENT_NAME", Value: "x", IsSecret: false, Source: EnvSourcePlatformMeta},
		{Role: RoleAgent, EnvName: "ANTHROPIC_API_KEY", Value: "sk", IsSecret: true, Source: EnvSourceUserVar},
		{Role: RoleMessaging, EnvName: "GRPC_ENABLED", Value: "true", IsSecret: false, Source: EnvSourceDerived},
	}
	cm, sec := Project(rows, RoleAgent)
	wantCM := map[string]string{"ASTRO_AGENT_NAME": "x"}
	wantSec := map[string]string{"ANTHROPIC_API_KEY": "sk"}
	if !reflect.DeepEqual(cm, wantCM) {
		t.Errorf("cm: got %v, want %v", cm, wantCM)
	}
	if !reflect.DeepEqual(sec, wantSec) {
		t.Errorf("sec: got %v, want %v", sec, wantSec)
	}
}

func TestProject_FiltersToOneRole(t *testing.T) {
	rows := []Resolution{
		{Role: RoleAgent, EnvName: "X", Value: "1"},
		{Role: RoleMessaging, EnvName: "Y", Value: "2"},
		{Role: KnowledgeRole("postgres"), EnvName: "Z", Value: "3"},
	}
	cm, _ := Project(rows, RoleMessaging)
	if _, ok := cm["X"]; ok {
		t.Error("agent's X leaked into messaging projection")
	}
	if _, ok := cm["Z"]; ok {
		t.Error("knowledge:postgres's Z leaked into messaging projection")
	}
	if cm["Y"] != "2" {
		t.Errorf("messaging Y missing: %v", cm)
	}
}

// P17: an empty resolution list produces nil (not empty) maps for both buckets.
func TestProject_P17_EmptyResolutionProducesEmptyMaps(t *testing.T) {
	cm, sec := Project(nil, RoleAgent)
	if len(cm) != 0 {
		t.Errorf("P17: cm should be empty, got %v", cm)
	}
	if len(sec) != 0 {
		t.Errorf("P17: sec should be empty, got %v", sec)
	}
}

func TestRolesIn_ReturnsDistinctRoles(t *testing.T) {
	rows := []Resolution{
		{Role: RoleAgent, EnvName: "A"},
		{Role: RoleMessaging, EnvName: "B"},
		{Role: RoleAgent, EnvName: "C"},
		{Role: KnowledgeRole("p"), EnvName: "D"},
	}
	got := RolesIn(rows)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	want := []Role{RoleAgent, KnowledgeRole("p"), RoleMessaging}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
