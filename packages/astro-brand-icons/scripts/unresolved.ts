#!/usr/bin/env bun
/**
 * Rank the outbound domains that have no brand icon yet.
 *
 * Input is the JSON from `queen <env> icons domains --json`. Resolution has to
 * happen here rather than server-side: the rules read this package's manifest,
 * and a second implementation in Go is how the alias tables drifted apart
 * before.
 *
 * Usage:
 *   queen prod icons domains --json > /tmp/domains.json
 *   bun scripts/unresolved.ts /tmp/domains.json
 *   bun scripts/unresolved.ts /tmp/domains.json --json
 */

import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const PKG_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");

interface IconEntry {
  id: string;
  domains?: string[];
  aliases?: string[];
}

interface OutboundDomain {
  domain?: string;
  request_count?: number;
  deployment_count?: number;
  hosts?: string[];
}

// Hosts that are never worth an icon: bare addresses, single-label internal
// names, and in-cluster service DNS.
function isNoise(domain: string): boolean {
  if (/^[\d.]+$/.test(domain) || domain.includes(":")) return true;
  if (!domain.includes(".")) return true;
  return domain.endsWith(".svc.cluster.local") || domain.endsWith(".local");
}

function buildIndex(icons: IconEntry[]): Map<string, string> {
  const index = new Map<string, string>();
  for (const icon of icons) {
    index.set(icon.id.toLowerCase(), icon.id);
    for (const key of [...(icon.domains ?? []), ...(icon.aliases ?? [])]) {
      index.set(key.toLowerCase(), icon.id);
    }
  }
  return index;
}

/** Mirrors iconIdForHost in astro-client: exact registrable domain, first label, then every parent. */
function resolveDomain(index: Map<string, string>, domain: string): string | null {
  const key = domain.toLowerCase();
  const direct = index.get(key) ?? index.get(key.split(".")[0]);
  if (direct) return direct;
  // Wildcard suffixes sit several labels up: a bucket at
  // mybucket.nyc3.digitaloceanspaces.com needs two steps to reach the platform.
  const parts = key.split(".");
  for (let i = 1; parts.length - i >= 2; i++) {
    const id = index.get(parts.slice(i).join("."));
    if (id) return id;
  }
  return null;
}

const args = process.argv.slice(2);
const asJson = args.includes("--json");
const inputPath = args.find((a) => !a.startsWith("--"));
if (!inputPath) {
  console.error("usage: bun scripts/unresolved.ts <domains.json> [--json]");
  process.exit(1);
}

const manifest = JSON.parse(
  readFileSync(resolve(PKG_ROOT, "icons.json"), "utf8"),
) as { icons: IconEntry[] };
const index = buildIndex(manifest.icons);

const payload = JSON.parse(readFileSync(resolve(inputPath), "utf8")) as {
  domains?: OutboundDomain[];
  window?: string;
};
const rows = payload.domains ?? [];

const scored = rows.filter((r) => r.domain && !isNoise(r.domain));
const noiseCount = rows.length - scored.length;

const unresolved = scored
  .filter((r) => !resolveDomain(index, r.domain!))
  .map((r) => ({
    domain: r.domain!,
    requests: r.request_count ?? 0,
    deployments: r.deployment_count ?? 0,
    hosts: r.hosts ?? [],
  }))
  .sort((a, b) => b.deployments - a.deployments || b.requests - a.requests);

if (asJson) {
  console.log(JSON.stringify({ window: payload.window, unresolved }, null, 2));
  process.exit(0);
}

console.log(`window ${payload.window ?? "?"} · ${manifest.icons.length} icons shipped`);
// Noise is counted out loud: it occupies server-side top-N slots, so a large
// number here means real domains fell off the end of --limit.
console.log(
  `${scored.length - unresolved.length} domains resolve, ${unresolved.length} do not` +
    (noiseCount > 0 ? ` (${noiseCount} of ${rows.length} discarded as noise)` : "") +
    "\n",
);

if (unresolved.length > 0) {
  const width = Math.max(...unresolved.map((u) => u.domain.length), 6);
  console.log(`${"DOMAIN".padEnd(width)}  DEPLOYMENTS  REQUESTS  EXAMPLE HOST`);
  for (const u of unresolved) {
    const host = u.hosts[0] && u.hosts[0] !== u.domain ? u.hosts[0] : "";
    console.log(
      `${u.domain.padEnd(width)}  ${String(u.deployments).padStart(11)}  ${String(u.requests).padStart(8)}  ${host}`,
    );
  }
}
