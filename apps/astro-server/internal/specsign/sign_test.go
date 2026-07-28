package specsign

import (
	"testing"

	spec "github.com/astropods/astro-spec"
)

func testSpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Account:  "acme",
			Name:     "my-agent",
			Build:    "abc123",
			Registry: "registry.example.com",
		},
		Target: spec.DeploymentTarget{
			Account:     "acme",
			DisplayName: "My Agent",
		},
		Agent: spec.DeploymentAgent{
			Image: "registry.example.com/acme/my-agent:abc123",
			Endpoints: map[string]spec.Endpoint{
				"http": {Port: 8080, Protocol: "http"},
			},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {
				Image:     "postgres:16",
				Endpoints: map[string]spec.Endpoint{"tcp": {Port: 5432, Protocol: "tcp"}},
			},
		},
		Variables: map[string]spec.Variable{
			"API_KEY": {Value: "secret", Secret: true},
		},
	}
}

func TestSignVerify_RoundTrip(t *testing.T) {
	key := NewKey()
	ds := testSpec()
	sig := Sign(key, ds)
	if !Verify(key, ds, sig) {
		t.Fatal("expected signature to verify")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	ds := testSpec()
	sig := Sign(NewKey(), ds)
	if Verify(NewKey(), ds, sig) {
		t.Fatal("expected verification to fail with wrong key")
	}
}

func TestVerify_TamperedImage(t *testing.T) {
	key := NewKey()
	ds := testSpec()
	sig := Sign(key, ds)
	ds.Agent.Image = "evil:latest"
	if Verify(key, ds, sig) {
		t.Fatal("expected verification to fail after image change")
	}
}

func TestVerify_TamperedBinding(t *testing.T) {
	key := NewKey()
	ds := testSpec()
	ds.Knowledge = map[string]spec.DeploymentKnowledge{
		"pg": {Binding: "arn:knowledge-store:acct:store1"},
	}
	sig := Sign(key, ds)
	k := ds.Knowledge["pg"]
	k.Binding = "arn:knowledge-store:acct:store2"
	ds.Knowledge["pg"] = k
	if Verify(key, ds, sig) {
		t.Fatal("expected verification to fail after binding change")
	}
}

func TestVerify_TargetFieldsIgnored(t *testing.T) {
	key := NewKey()
	ds := testSpec()
	sig := Sign(key, ds)

	// Client changes target fields after receiving template — should still verify.
	ds.Target.Account = "other-account"
	ds.Target.DisplayName = "Different Name"
	ds.Target.DeploymentID = "deploy-123"
	ds.Target.ClusterID = "eu-west-1-managed"
	if !Verify(key, ds, sig) {
		t.Fatal("expected signature to still verify after target field changes")
	}
}

func TestVerify_EmptySignature(t *testing.T) {
	key := NewKey()
	ds := testSpec()
	if Verify(key, ds, "") {
		t.Fatal("expected empty signature to fail")
	}
}

func TestVerify_MalformedSignature(t *testing.T) {
	key := NewKey()
	ds := testSpec()
	if Verify(key, ds, "not-hex!") {
		t.Fatal("expected malformed signature to fail")
	}
}

func TestNewKey_Unique(t *testing.T) {
	k1 := NewKey()
	k2 := NewKey()
	if string(k1) == string(k2) {
		t.Fatal("expected unique keys")
	}
}
