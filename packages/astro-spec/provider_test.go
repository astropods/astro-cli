package spec

import (
	"reflect"
	"testing"
)

func TestLookupBuiltin_MySQL(t *testing.T) {
	p, ok := LookupBuiltin("knowledge", "mysql")
	if !ok {
		t.Fatal("expected mysql to be registered under section 'knowledge'")
	}

	if p.Cloud {
		t.Error("mysql should not be a cloud provider")
	}
	if p.DefaultPort != 3306 {
		t.Errorf("DefaultPort = %d, want 3306", p.DefaultPort)
	}
	if p.EnvPrefix != "MYSQL" {
		t.Errorf("EnvPrefix = %q, want %q", p.EnvPrefix, "MYSQL")
	}
	if p.Image == "" {
		t.Error("Image should be set so managed mode can be enabled later")
	}
	if p.MountPath != "/var/lib/mysql" {
		t.Errorf("MountPath = %q, want %q", p.MountPath, "/var/lib/mysql")
	}

	wantBindCreds := []BindCredentialDef{
		{Attr: "user", StorageKey: "MYSQL_USER"},
		{Attr: "password", StorageKey: "MYSQL_PASSWORD"},
		{Attr: "database", StorageKey: "MYSQL_DATABASE"},
	}
	if !reflect.DeepEqual(p.BindCredentials, wantBindCreds) {
		t.Errorf("BindCredentials = %+v, want %+v", p.BindCredentials, wantBindCreds)
	}
}

func TestCredentialStorageKeyMap_MySQL(t *testing.T) {
	got := CredentialStorageKeyMap("mysql")
	want := map[string]string{
		"MYSQL_USER":     "user",
		"MYSQL_PASSWORD": "password",
		"MYSQL_DATABASE": "database",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CredentialStorageKeyMap(mysql) = %+v, want %+v", got, want)
	}
}

func TestIsGatewayModelProvider(t *testing.T) {
	for _, name := range []string{"gateway", "Gateway", "GATEWAY"} {
		if !IsGatewayModelProvider(name) {
			t.Errorf("IsGatewayModelProvider(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"", "anthropic", "openai", "gatewayx"} {
		if IsGatewayModelProvider(name) {
			t.Errorf("IsGatewayModelProvider(%q) = true, want false", name)
		}
	}
	// gateway is not a builtin container/cloud provider.
	if _, ok := LookupBuiltin("models", "gateway"); ok {
		t.Error("gateway must not be a builtin provider entry")
	}
	if IsCloudModelProvider("gateway") {
		t.Error("gateway must not be a cloud model provider")
	}
}

func TestGatewayModel_Semantics(t *testing.T) {
	m := Model{Provider: "gateway", Models: []string{"claude-sonnet-4-6", "gpt-4o"}}
	if !m.IsProviderMode() {
		t.Error("gateway model should be provider mode")
	}
	if m.DeploysContainer(nil) {
		t.Error("gateway model must not deploy a container")
	}
	if rc := m.ResolvedContainer(); rc.Image != "" || rc.Port != 0 {
		t.Errorf("gateway ResolvedContainer() = %+v, want zero value", rc)
	}
}
