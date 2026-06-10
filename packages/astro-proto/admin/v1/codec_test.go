package adminv1

import "testing"

// Regression test for the codec name collision documented in
// docs/changelog/fix/grpc-codec-name-collision-2026-06-10.md.
//
// jsonCodec is registered globally via init(). If its Name() returns "proto",
// it shadows gRPC's default proto codec process-wide for every binary that
// imports this package — including any embedded gRPC server (BuildKit's
// session grpc_health_v1.HealthServer was the original blast radius). The
// codec MUST stay named "json"; clients opt in via grpc.CallContentSubtype.
func TestCodecNameIsJSON(t *testing.T) {
	got := (jsonCodec{}).Name()
	if got != "json" {
		t.Fatalf(
			"jsonCodec.Name() = %q; want %q.\n\n"+
				"Renaming this codec to %q would shadow gRPC's default proto codec "+
				"process-wide and break every embedded gRPC server in any binary that "+
				"imports astro-proto. See "+
				"docs/changelog/fix/grpc-codec-name-collision-2026-06-10.md.",
			got, "json", "proto",
		)
	}
}
