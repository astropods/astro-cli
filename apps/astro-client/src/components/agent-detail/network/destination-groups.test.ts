import { describe, expect, it } from "vitest";
import {
  destinationLabel,
  groupDestinations,
  groupIconId,
  hostOf,
  iconIdForPeer,
  routeLabel,
} from "./destination-groups";
import type { NetworkFlow } from "@/lib/api";

/** `registrableDomain` mirrors the server's eTLD+1; omit it to model a peer it can't resolve. */
function flow(
  peer: string,
  requestCount: number,
  registrableDomain?: string,
  bytesTotal = 0,
): NetworkFlow {
  return {
    peer,
    peer_kind: "address",
    request_count: requestCount,
    error_count: 0,
    error_rate: 0,
    latency_p50_ms: null,
    latency_p95_ms: null,
    bytes_total: bytesTotal,
    registrable_domain: registrableDomain,
  };
}

const domainsOf = (flows: NetworkFlow[]) => groupDestinations(flows).map((g) => g.domain);

describe("groupDestinations", () => {
  it("merges hosts sharing a registrable domain", () => {
    const groups = groupDestinations([
      flow("api.slack.com", 10, "slack.com"),
      flow("hooks.slack.com", 30, "slack.com"),
      flow("files.slack.com", 5, "slack.com"),
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].domain).toBe("slack.com");
    expect(groups[0].requestCount).toBe(45);
  });

  it("sums request counts and bytes across the merged hosts", () => {
    const [group] = groupDestinations([
      flow("api.acme.io", 10, "acme.io", 1_000),
      flow("cdn.acme.io", 4, "acme.io", 250),
    ]);

    expect(group.requestCount).toBe(14);
    expect(group.bytesTotal).toBe(1_250);
  });

  it("orders constituent hosts by request count, busiest first", () => {
    const [group] = groupDestinations([
      flow("c.acme.io", 5, "acme.io"),
      flow("a.acme.io", 50, "acme.io"),
      flow("b.acme.io", 20, "acme.io"),
    ]);

    expect(group.hosts.map((h) => h.peer)).toEqual(["a.acme.io", "b.acme.io", "c.acme.io"]);
  });

  it("keeps different registrable domains apart", () => {
    expect(
      domainsOf([flow("api.acme.com", 1, "acme.com"), flow("api.acme.io", 1, "acme.io")]),
    ).toEqual(["acme.com", "acme.io"]);
  });

  // The server leaves this empty for anything that isn't a registrable domain.
  it("falls back to the host when the server sends no domain", () => {
    expect(domainsOf([flow("10.0.14.22", 1), flow("internal-svc", 1)])).toEqual([
      "10.0.14.22",
      "internal-svc",
    ]);
  });

  it("strips ports in the fallback so one host on two ports is one destination", () => {
    const groups = groupDestinations([flow("api.acme.io:443", 3), flow("api.acme.io:8080", 4)]);

    expect(groups).toHaveLength(1);
    expect(groups[0].requestCount).toBe(7);
  });
});

describe("iconIdForPeer", () => {
  it("resolves an outbound host from the server's registrable domain", () => {
    expect(iconIdForPeer("api.openai.com", "outbound", "openai.com")).toBe("openai");
  });

  // Only reachable through DOMAIN_TO_ICON: "hubapi" is not an icon id.
  it("resolves hosts whose domain does not carry the vendor name", () => {
    expect(iconIdForPeer("api.hubapi.com", "outbound", "hubapi.com")).toBe("hubspot");
  });

  // Hosting platforms are public suffixes, so eTLD+1 keeps the customer's
  // label and the platform only appears in the parent.
  it("resolves a hosting platform from the parent domain", () => {
    expect(iconIdForPeer("myapp.vercel.app", "outbound", "myapp.vercel.app")).toBe("vercel");
    expect(iconIdForPeer("raw.githubusercontent.com", "outbound", "raw.githubusercontent.com")).toBe("github");
  });

  // Wildcard PSL rules put the platform two or more labels up.
  it("walks past a wildcard suffix to reach the platform", () => {
    const bucket = "mybucket.nyc3.digitaloceanspaces.com";
    expect(iconIdForPeer(bucket, "outbound", bucket)).toBe("digitalocean");
  });

  it("does not climb past the registrable domain for ordinary hosts", () => {
    expect(iconIdForPeer("api.acme-cdn.io", "outbound", "acme-cdn.io")).toBeNull();
  });

  it("resolves nothing without a registrable domain", () => {
    expect(iconIdForPeer("api.openai.com", "outbound")).toBeNull();
  });

  it("returns null for an outbound host with no shipped icon", () => {
    expect(iconIdForPeer("api.acme-cdn.io", "outbound", "acme-cdn.io")).toBeNull();
  });

  it("never resolves inbound routes, which carry no brand", () => {
    expect(iconIdForPeer("/api/chat", "inbound")).toBeNull();
    expect(iconIdForPeer("/metrics", "inbound")).toBeNull();
  });

  // Beyla emits OTel's `postgresql`, but the icon ships as `postgres`.
  it("aliases the OTel postgres identifier onto the shipped icon id", () => {
    expect(iconIdForPeer("postgresql", "database")).toBe("postgres");
  });

  it("passes through db systems whose name already matches an icon", () => {
    expect(iconIdForPeer("redis", "database")).toBe("redis");
    expect(iconIdForPeer("mongodb", "database")).toBe("mongodb");
  });

  it("returns null for a db system with no shipped icon", () => {
    expect(iconIdForPeer("cassandra", "database")).toBeNull();
  });

  // Nothing guarantees the casing Beyla reports.
  it("folds case", () => {
    expect(iconIdForPeer("PostgreSQL", "database")).toBe("postgres");
    expect(iconIdForPeer("API.OpenAI.com", "outbound", "openai.com")).toBe("openai");
  });

  it("does not resolve inherited object keys as icons", () => {
    expect(iconIdForPeer("__proto__", "outbound", "__proto__")).toBeNull();
    expect(iconIdForPeer("constructor", "outbound", "constructor")).toBeNull();
    expect(iconIdForPeer("constructor", "database")).toBeNull();
    expect(iconIdForPeer("__proto__", "database")).toBeNull();
  });
});

describe("groupIconId", () => {
  it("resolves a merged group from its hosts", () => {
    const [group] = groupDestinations([
      flow("hooks.slack.com", 30, "slack.com"),
      flow("files.slack.com", 5, "slack.com"),
    ]);
    expect(groupIconId(group)).toBe("slack");
  });

  it("returns null when no host in the group ships an icon", () => {
    const [group] = groupDestinations([flow("shard-0.edge.acme-cdn.io", 10, "acme-cdn.io")]);
    expect(groupIconId(group)).toBeNull();
  });

  // Exercises the loop's early return rather than the first-host-wins path.
  it("resolves from a later host when the busiest one has no icon", () => {
    const [group] = groupDestinations([
      flow("cdn.hubapi.com", 100, "hubapi.com"),
      flow("api.hubapi.com", 5, "hubapi.com"),
    ]);
    expect(group.hosts[0].peer).toBe("cdn.hubapi.com");
    expect(groupIconId(group)).toBe("hubspot");
  });
});

describe("hostOf", () => {
  it("strips scheme and port", () => {
    expect(hostOf("https://api.acme.io:443/v1")).toBe("api.acme.io");
    expect(hostOf("api.acme.io:8080")).toBe("api.acme.io");
  });

  // These used to parse as a scheme and return an empty hostname.
  it("does not mistake an http-prefixed host for a scheme", () => {
    expect(hostOf("httpbin.org:443")).toBe("httpbin.org");
    expect(hostOf("http-gateway:8080")).toBe("http-gateway");
  });

  // Falls back to the raw peer; "" would collapse unrelated destinations.
  it("returns the raw peer when it cannot be parsed", () => {
    expect(hostOf("has space:80")).toBe("has space:80");
  });

  it("returns the raw peer for a scheme that carries no host", () => {
    expect(hostOf("unix:///var/run/agent.sock")).toBe("unix:///var/run/agent.sock");
  });
});

describe("destinationLabel", () => {
  it("uses the vendor label of the group key", () => {
    expect(destinationLabel("acme-corp.io", 20)).toBe("acme-corp");
    expect(destinationLabel("example.co.uk", 20)).toBe("example");
  });

  it("clips a long label from the end", () => {
    expect(destinationLabel("verylongvendorname.com", 6)).toBe("veryl…");
  });

  it("keeps an address whole when it fits", () => {
    expect(destinationLabel("10.0.14.22", 12)).toBe("10.0.14.22");
  });

  it("keeps the low-order octets of an address that does not fit", () => {
    expect(destinationLabel("10.0.14.22", 7)).toBe("….14.22");
  });

  it("passes single-label hosts through", () => {
    expect(destinationLabel("internal-svc", 20)).toBe("internal-svc");
  });
});

describe("routeLabel", () => {
  it("shows the whole route when it fits, minus the leading slash", () => {
    expect(routeLabel("/api/chat", 20)).toBe("api/chat");
  });

  it("falls back to the last segment", () => {
    expect(routeLabel("/api/tools/invoke", 9)).toBe("invoke");
  });

  it("clips the last segment from the end when it still does not fit", () => {
    expect(routeLabel("/api/sessions", 6)).toBe("sessi…");
  });

  it("handles the root route", () => {
    expect(routeLabel("/", 6)).toBe("/");
  });
});

/** A label has to fit the chord it was measured against, or it overflows. */
describe("label budgets", () => {
  const DOMAINS = [
    "acme-corp.io",
    "example.co.uk",
    "10.0.14.22",
    "internal-svc",
    "verylongvendorname.com",
  ];
  const ROUTES = ["/", "/api/chat", "/api/tools/invoke", "/healthz", "/api/a/b/c/deeply/nested"];

  it("fits every label into its budget, and never empties it", () => {
    for (let maxChars = 2; maxChars <= 20; maxChars++) {
      for (const domain of DOMAINS) {
        const label = destinationLabel(domain, maxChars);
        const where = `destinationLabel(${domain}, ${maxChars}) = ${label}`;
        expect(label.length, where).toBeLessThanOrEqual(maxChars);
        expect(label.length, where).toBeGreaterThan(0);
      }
      for (const route of ROUTES) {
        const label = routeLabel(route, maxChars);
        const where = `routeLabel(${route}, ${maxChars}) = ${label}`;
        expect(label.length, where).toBeLessThanOrEqual(maxChars);
        expect(label.length, where).toBeGreaterThan(0);
      }
    }
  });
});
