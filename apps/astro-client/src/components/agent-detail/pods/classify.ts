/**
 * Single source of truth for what a pod tile is. A tile's role is derived from
 * its `component` label (`agent`, `collector`, `knowledge-<name>`,
 * `model-<name>`, `tool-<name>`, `ingestion-<name>`), and drives both its icon
 * and its place in the layout.
 */

import { hasIntegrationIcon } from "@/lib/integration-icon-ids";

export type Role =
  | "agent"
  | "knowledge"
  | "model"
  | "integration"
  | "ingestion"
  | "collector"
  | "other";

// Bare provider names, for workloads labelled by provider rather than the
// `<role>-<name>` convention.
const KNOWLEDGE_PROVIDERS = new Set(["postgres", "mysql", "redis", "qdrant", "neo4j", "pinecone"]);

export function classify(component: string | undefined, kind?: string): Role {
  const c = (component ?? "").toLowerCase();
  if (c === "agent") return "agent";
  if (c === "collector") return "collector";
  if (c.startsWith("knowledge-")) return "knowledge";
  if (c.startsWith("model-")) return "model";
  if (c.startsWith("tool-")) return "integration";
  if (c.startsWith("ingestion-")) return "ingestion";
  if (KNOWLEDGE_PROVIDERS.has(c)) return "knowledge";
  // Runtime-only manual ingestion firings arrive as bare Jobs/CronJobs.
  if (kind === "Job" || kind === "CronJob") return "ingestion";
  return "other";
}

// Left-to-right flow of the layout, and the order tiles stack within a column.
const ROLE_SEQUENCE: Role[] = [
  "ingestion",
  "knowledge",
  "agent",
  "model",
  "integration",
  "collector",
  "other",
];

export function roleRank(role: Role): number {
  const i = ROLE_SEQUENCE.indexOf(role);
  return i === -1 ? ROLE_SEQUENCE.length : i;
}

/**
 * The first-party brand-icon id for a knowledge/model/integration tile, or null
 * (any other role, or no shipped icon — caller uses the role icon). Prefers the
 * declared `provider` (the platform's own identity, e.g. `postgres`); falls back
 * to the component's provider suffix for custom containers with no provider.
 */
export function brandIconId(role: Role, provider: string | undefined, component: string | undefined): string | null {
  if (role !== "knowledge" && role !== "model" && role !== "integration") return null;
  const p = (provider ?? "").toLowerCase();
  if (p && hasIntegrationIcon(p)) return p;
  const suffix = (component ?? "").toLowerCase().replace(/^(knowledge|model|tool)-/, "");
  return hasIntegrationIcon(suffix) ? suffix : null;
}
