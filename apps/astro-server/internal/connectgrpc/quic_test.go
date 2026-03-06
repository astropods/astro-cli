package connectgrpc

import (
	"encoding/base64"
	"testing"
)

func TestParseNLBServerID_Valid(t *testing.T) {
	// LWV1oW+gjqQ= is the real value from the AWS LBC (8 bytes base64-encoded)
	t.Setenv("AWS_LBC_QUIC_SERVER_ID", "LWV1oW+gjqQ=")

	id, ok := parseNLBServerID()
	if !ok {
		t.Fatal("expected parseNLBServerID to return ok=true")
	}

	decoded, _ := base64.StdEncoding.DecodeString("LWV1oW+gjqQ=")
	for i := range id {
		if id[i] != decoded[i] {
			t.Fatalf("byte %d: got %02x, want %02x", i, id[i], decoded[i])
		}
	}
}

func TestParseNLBServerID_Empty(t *testing.T) {
	t.Setenv("AWS_LBC_QUIC_SERVER_ID", "")

	_, ok := parseNLBServerID()
	if ok {
		t.Fatal("expected ok=false for empty env var")
	}
}

func TestParseNLBServerID_Unset(t *testing.T) {
	// Don't set the env var at all
	_, ok := parseNLBServerID()
	if ok {
		t.Fatal("expected ok=false when env var is unset")
	}
}

func TestParseNLBServerID_InvalidBase64(t *testing.T) {
	t.Setenv("AWS_LBC_QUIC_SERVER_ID", "not-valid-base64!!!")

	_, ok := parseNLBServerID()
	if ok {
		t.Fatal("expected ok=false for invalid base64")
	}
}

func TestParseNLBServerID_WrongLength(t *testing.T) {
	// Only 4 bytes (not 8)
	t.Setenv("AWS_LBC_QUIC_SERVER_ID", base64.StdEncoding.EncodeToString([]byte{1, 2, 3, 4}))

	_, ok := parseNLBServerID()
	if ok {
		t.Fatal("expected ok=false for 4-byte value (need exactly 8)")
	}
}

func TestQuicLBConnIDGenerator_Length(t *testing.T) {
	g := &quicLBConnIDGenerator{serverID: [8]byte{0x2d, 0x65, 0x75, 0xa1, 0x6f, 0xa0, 0x8e, 0xa4}}

	if g.ConnectionIDLen() != 13 {
		t.Fatalf("ConnectionIDLen: got %d, want 13", g.ConnectionIDLen())
	}
}

func TestQuicLBConnIDGenerator_Format(t *testing.T) {
	serverID := [8]byte{0x2d, 0x65, 0x75, 0xa1, 0x6f, 0xa0, 0x8e, 0xa4}
	g := &quicLBConnIDGenerator{serverID: serverID}

	cid, err := g.GenerateConnectionID()
	if err != nil {
		t.Fatalf("GenerateConnectionID: %v", err)
	}

	raw := cid.Bytes()

	// Total length must be 13
	if len(raw) != 13 {
		t.Fatalf("CID length: got %d, want 13", len(raw))
	}

	// Byte 0: config rotation header (top 3 bits = 0 for config rotation 0)
	if raw[0]&0xE0 != 0x00 {
		t.Fatalf("byte 0 config rotation bits: got %08b, want top 3 bits = 000", raw[0])
	}

	// Bytes 1-8: must be the server ID
	for i := 0; i < 8; i++ {
		if raw[1+i] != serverID[i] {
			t.Fatalf("server ID byte %d: got %02x, want %02x", i, raw[1+i], serverID[i])
		}
	}

	// Bytes 9-12: nonce (just check they exist, can't predict random values)
	// But verify two generated CIDs have different nonces
	cid2, _ := g.GenerateConnectionID()
	raw2 := cid2.Bytes()
	if string(raw[9:]) == string(raw2[9:]) {
		t.Fatal("nonce bytes should differ between generated CIDs")
	}
}

func TestQuicLBConnIDGenerator_ServerIDAtCorrectOffset(t *testing.T) {
	// The NLB extracts bytes 1-8 as the server ID.
	// Verify with multiple different server IDs.
	tests := []struct {
		name     string
		serverID [8]byte
	}{
		{"zeros", [8]byte{}},
		{"ones", [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{"real", [8]byte{0x2d, 0x65, 0x75, 0xa1, 0x6f, 0xa0, 0x8e, 0xa4}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &quicLBConnIDGenerator{serverID: tt.serverID}
			cid, err := g.GenerateConnectionID()
			if err != nil {
				t.Fatal(err)
			}
			raw := cid.Bytes()

			// Extract bytes 1-8 (what the NLB reads)
			var extracted [8]byte
			copy(extracted[:], raw[1:9])

			if extracted != tt.serverID {
				t.Fatalf("NLB would extract server ID %x, want %x", extracted, tt.serverID)
			}
		})
	}
}
