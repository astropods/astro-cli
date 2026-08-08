import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { NetworkFlowsTable } from "./NetworkFlowsTable";
import type { NetworkDirection, NetworkFlow } from "@/lib/api";

afterEach(cleanup);

function flow(peer: string, peerKind: NetworkFlow["peer_kind"] = "address"): NetworkFlow {
  return {
    peer,
    peer_kind: peerKind,
    registrable_domain:
      peerKind === "address" ? peer.split(".").slice(-2).join(".") : undefined,
    request_count: 100,
    error_count: 0,
    error_rate: 0,
    latency_p50_ms: 10,
    latency_p95_ms: 40,
    bytes_total: 2_048,
  };
}

/** The brand `<img>` in the row whose peer cell reads `peer`, if any. */
function iconFor(peer: string): HTMLImageElement | null {
  const row = screen.getByText(peer).closest("tr");
  if (!row) throw new Error(`no row for ${peer}`);
  return row.querySelector("img");
}

function renderTable(direction: NetworkDirection, flows: NetworkFlow[]) {
  render(<NetworkFlowsTable direction={direction} flows={flows} />);
}

describe("NetworkFlowsTable peer icons", () => {
  it("prefixes outbound hosts with their brand icon", () => {
    renderTable("outbound", [flow("api.openai.com"), flow("hooks.slack.com")]);

    expect(iconFor("api.openai.com")?.getAttribute("src")).toContain("openai");
    expect(iconFor("hooks.slack.com")?.getAttribute("src")).toContain("slack");
  });

  it("leaves an outbound host with no shipped icon unprefixed", () => {
    renderTable("outbound", [flow("api.acme-cdn.io")]);

    expect(iconFor("api.acme-cdn.io")).toBeNull();
  });

  // The slot should collapse, not indent every row behind an empty gutter.
  it("never prefixes inbound routes", () => {
    renderTable("inbound", [flow("/api/chat", "route"), flow("/webhooks/slack", "route")]);

    expect(iconFor("/api/chat")).toBeNull();
    expect(iconFor("/webhooks/slack")).toBeNull();
  });

  it("prefixes database rows, aliasing the OTel postgres name", () => {
    renderTable("database", [flow("postgresql", "db_system"), flow("redis", "db_system")]);

    expect(iconFor("postgresql")?.getAttribute("src")).toContain("postgres");
    expect(iconFor("redis")?.getAttribute("src")).toContain("redis");
  });

});
