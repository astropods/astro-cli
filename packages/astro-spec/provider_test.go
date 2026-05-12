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
