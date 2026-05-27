package deploymentstore

import (
	"encoding/base64"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

func TestIsInlineSecret(t *testing.T) {
	if !IsInlineSecret(Variable{Secret: true, Value: "e30="}) {
		t.Fatal("expected inline secret")
	}
	if IsInlineSecret(Variable{Secret: true, Ref: "OTHER"}) {
		t.Fatal("vault ref is not inline")
	}
	if IsInlineSecret(Variable{Secret: true, Value: ""}) {
		t.Fatal("empty value is not inline")
	}
}

func TestInlineSecretNames(t *testing.T) {
	stored := []Variable{
		{Name: "API_KEY", Secret: true, Value: "e30="},
		{Name: "LOG_LEVEL", Secret: false, Value: "info"},
		{Name: "OTHER", Secret: true, Ref: "X"},
	}
	names := InlineSecretNames(stored)
	if len(names) != 1 || names[0] != "API_KEY" {
		t.Fatalf("names = %v, want [API_KEY]", names)
	}
}

func TestPlaintextValue_NoKMS(t *testing.T) {
	val, ok := PlaintextValue(nil, Variable{Value: "c2VjcmV0"})
	if !ok || val != "secret" {
		t.Fatalf("got %q, %v", val, ok)
	}
}

func TestPlaintextValue_Encrypted(t *testing.T) {
	enc, err := envelope.NewTestEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := enc.Encrypt([]byte("top-secret"))
	if err != nil {
		t.Fatal(err)
	}
	dec, err := envelope.NewTestDecryptor(make([]byte, 32), enc.EncryptedDataKey)
	if err != nil {
		t.Fatal(err)
	}
	val, ok := PlaintextValue(dec, Variable{
		Value: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce: nonce,
	})
	if !ok || val != "top-secret" {
		t.Fatalf("got %q, %v", val, ok)
	}
}
