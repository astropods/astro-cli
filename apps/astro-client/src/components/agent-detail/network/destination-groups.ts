import { resolveIconId } from "@/lib/integration-icon-ids";
import type { NetworkDirection, NetworkFlow } from "@/lib/api";

/** Hosts of one vendor merged onto a single bubble; `hosts` keeps the originals. */
export interface DestinationGroup {
  domain: string;
  /** Most requests first. Never empty. */
  hosts: NetworkFlow[];
  requestCount: number;
  bytesTotal: number;
}

const IPV4 = /^\d{1,3}(\.\d{1,3}){3}$/;

/** Keys off `://`, not an `http` prefix: `httpbin.org:443` is a host, not a scheme. */
export function hostOf(peer: string): string {
  try {
    const { hostname } = new URL(peer.includes("://") ? peer : `https://${peer}`);
    return hostname || peer;
  } catch {
    return peer;
  }
}

/**
 * What a flow groups under. eTLD+1 is registry policy, not something derivable
 * from a hostname, so the server computes it against the public suffix list;
 * peers it leaves empty (bare IPs, single-label names) stand alone.
 */
function groupKey(flow: NetworkFlow): string {
  return flow.registrable_domain || hostOf(flow.peer);
}

/** `slack.com` → `slack`. */
function vendorLabel(domain: string): string {
  return domain.split(".")[0];
}

/** Percentiles are dropped rather than combined — p95s across hosts don't sum. */
export function groupDestinations(flows: NetworkFlow[]): DestinationGroup[] {
  const byDomain = new Map<string, DestinationGroup>();

  for (const flow of flows) {
    const domain = groupKey(flow);
    const existing = byDomain.get(domain);
    if (existing) {
      existing.hosts.push(flow);
      existing.requestCount += flow.request_count;
      existing.bytesTotal += flow.bytes_total;
    } else {
      byDomain.set(domain, {
        domain,
        hosts: [flow],
        requestCount: flow.request_count,
        bytesTotal: flow.bytes_total,
      });
    }
  }

  for (const group of byDomain.values()) {
    group.hosts.sort((a, b) => b.request_count - a.request_count);
  }
  return [...byDomain.values()];
}

/**
 * Hosting platforms like `vercel.app` are public suffixes, so eTLD+1 keeps the
 * customer's label and the platform never appears as the key. Falling back to
 * the parent lets `myapp.vercel.app` still resolve to Vercel.
 */
export function iconIdForHost(registrableDomain?: string): string | null {
  if (!registrableDomain) return null;
  const direct =
    resolveIconId(registrableDomain) ?? resolveIconId(vendorLabel(registrableDomain));
  if (direct) return direct;

  // Wildcard suffixes sit several labels up: a bucket at
  // mybucket.nyc3.digitaloceanspaces.com needs two steps to reach the platform.
  // Stops at two labels so a bare TLD can never match.
  const parts = registrableDomain.split(".");
  for (let i = 1; parts.length - i >= 2; i++) {
    const id = resolveIconId(parts.slice(i).join("."));
    if (id) return id;
  }
  return null;
}

export function iconIdForDbSystem(peer: string): string | null {
  return resolveIconId(peer);
}

/** Inbound peers are `http_route` templates, so there's no brand to resolve. */
export function iconIdForPeer(
  peer: string,
  direction: NetworkDirection,
  registrableDomain?: string,
): string | null {
  if (direction === "outbound") return iconIdForHost(registrableDomain);
  if (direction === "database") return iconIdForDbSystem(peer);
  return null;
}

/** Built on `iconIdForHost` so the graph and the flows table can't drift apart. */
export function groupIconId(group: DestinationGroup): string | null {
  for (const host of group.hosts) {
    const id = iconIdForHost(host.registrable_domain);
    if (id) return id;
  }
  return null;
}

/** Bubble text for an unbranded destination, taken from its group key. */
export function destinationLabel(domain: string, maxChars: number): string {
  if (IPV4.test(domain)) {
    // Reduced like a domain, 10.0.14.22 would label itself "10".
    if (domain.length <= maxChars) return domain;
    return `…${domain.slice(-Math.max(1, maxChars - 1))}`;
  }
  const label = vendorLabel(domain);
  if (label.length <= maxChars) return label;
  return `${label.slice(0, Math.max(1, maxChars - 1))}…`;
}

/** Bubble text for a route, falling back to the segment that varies. */
export function routeLabel(peer: string, maxChars: number): string {
  const trimmed = peer.length > 1 ? peer.replace(/^\//, "") : peer;
  if (trimmed.length <= maxChars) return trimmed;

  const lastSegment = trimmed.slice(trimmed.lastIndexOf("/") + 1);
  if (lastSegment.length <= maxChars) return lastSegment;
  return `${lastSegment.slice(0, Math.max(1, maxChars - 1))}…`;
}
