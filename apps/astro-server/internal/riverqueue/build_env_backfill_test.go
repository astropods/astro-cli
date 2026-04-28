package riverqueue

import (
	"reflect"
	"sort"
	"testing"
)

func TestRolesForLegacyTargets(t *testing.T) {
	cases := []struct {
		name           string
		targets        []string
		ingestionNames []string
		want           []string
	}{
		{
			name:    "agent only",
			targets: []string{"agent"},
			want:    []string{"agent"},
		},
		{
			name:    "interface adapter folds to messaging",
			targets: []string{"interface.slack"},
			want:    []string{"messaging"},
		},
		{
			name:    "multiple interface adapters still one messaging row",
			targets: []string{"interface.slack", "interface.web"},
			want:    []string{"messaging"},
		},
		{
			name:           "bare ingestion fans across all declared",
			targets:        []string{"ingestion"},
			ingestionNames: []string{"reporter", "backfill"},
			want:           []string{"ingestion:backfill", "ingestion:reporter"},
		},
		{
			name:    "qualified ingestion narrows to one",
			targets: []string{"ingestion.reporter"},
			want:    []string{"ingestion:reporter"},
		},
		{
			name:           "agent + ingestion combo",
			targets:        []string{"agent", "ingestion"},
			ingestionNames: []string{"reporter"},
			want:           []string{"agent", "ingestion:reporter"},
		},
		{
			name:    "unknown target produces no rows",
			targets: []string{"some-future-target"},
			want:    []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rolesForLegacyTargets(c.targets, c.ingestionNames)
			sort.Strings(got)
			want := append([]string{}, c.want...)
			sort.Strings(want)
			if len(got) == 0 && len(want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func TestDecodeLegacyValue(t *testing.T) {
	t.Run("non-secret stored plaintext", func(t *testing.T) {
		v, n, err := decodeLegacyValue("hello", nil, false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(v) != "hello" {
			t.Errorf("value: got %q, want %q", v, "hello")
		}
		if n != nil {
			t.Errorf("non-secret should have nil nonce, got %v", n)
		}
	})
	t.Run("secret base64-decoded with nonce passthrough", func(t *testing.T) {
		ciphertext := []byte{0x01, 0x02, 0x03}
		// Mimic the legacy storage shape: value column holds base64(ciphertext).
		encoded := "AQID"
		nonce := []byte{0x10, 0x11, 0x12}
		v, n, err := decodeLegacyValue(encoded, nonce, true)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !reflect.DeepEqual(v, ciphertext) {
			t.Errorf("value: got %v, want %v", v, ciphertext)
		}
		if !reflect.DeepEqual(n, nonce) {
			t.Errorf("nonce: got %v, want %v", n, nonce)
		}
	})
	t.Run("empty secret value with nonce", func(t *testing.T) {
		nonce := []byte{0xAA}
		v, n, err := decodeLegacyValue("", nonce, true)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(v) != 0 {
			t.Errorf("empty secret: expected zero-length value, got %v", v)
		}
		if !reflect.DeepEqual(n, nonce) {
			t.Errorf("empty secret: expected nonce %v, got %v", nonce, n)
		}
	})
	t.Run("malformed base64 returns error", func(t *testing.T) {
		_, _, err := decodeLegacyValue("not-base-64!!!", []byte{1}, true)
		if err == nil {
			t.Error("expected error for malformed base64")
		}
	})
	t.Run("secret stored plaintext when no KMS configured", func(t *testing.T) {
		// Without a KMS encryptor, SaveNormalizedSpec falls back to
		// storing secret values as plaintext with nil nonce. Backfill
		// must passthrough rather than try to base64-decode.
		v, n, err := decodeLegacyValue("xoxb-real-token", nil, true)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if string(v) != "xoxb-real-token" {
			t.Errorf("plaintext secret: got %q, want %q", v, "xoxb-real-token")
		}
		if n != nil {
			t.Errorf("plaintext secret: expected nil nonce, got %v", n)
		}
	})
}

func TestIngestionNamesFromSpec(t *testing.T) {
	t.Run("empty spec", func(t *testing.T) {
		names, err := ingestionNamesFromSpec("")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("got %v, want []", names)
		}
	})
	t.Run("spec with ingestion entries", func(t *testing.T) {
		spec := `{"ingestion":{"reporter":{},"backfill":{}}}`
		names, err := ingestionNamesFromSpec(spec)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		sort.Strings(names)
		want := []string{"backfill", "reporter"}
		if !reflect.DeepEqual(names, want) {
			t.Errorf("got %v, want %v", names, want)
		}
	})
	t.Run("spec without ingestion key", func(t *testing.T) {
		spec := `{"agent":{"image":"x"}}`
		names, err := ingestionNamesFromSpec(spec)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(names) != 0 {
			t.Errorf("got %v, want []", names)
		}
	})
	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := ingestionNamesFromSpec(`{not json`)
		if err == nil {
			t.Error("expected error for bad JSON")
		}
	})
}
