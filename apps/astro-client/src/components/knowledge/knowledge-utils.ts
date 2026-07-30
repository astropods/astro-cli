import type { KnowledgeProvider, KnowledgeStatus } from "@/lib/api";
import type { StatusBadgeColor } from "@/components/StatusBadge";

/**
 * A Supabase store is created as a plain "postgres" store; its Supabase origin
 * lives only in annotations.source. For display we surface that origin so the
 * store shows the Supabase icon/label instead of generic Postgres.
 */
export function displayProvider(store: {
  provider: KnowledgeProvider;
  annotations?: Record<string, string>;
}): KnowledgeProvider {
  return store.annotations?.source === "supabase" ? "supabase" : store.provider;
}

/**
 * Maps a Supabase service status (e.g. "ACTIVE_HEALTHY") to a human-friendly
 * label. Unknown statuses are humanized (ACTIVE_HEALTHY → "Active healthy");
 * when no status is present, falls back to the boolean healthy flag.
 */
const SUPABASE_HEALTH_LABELS: Record<string, string> = {
  ACTIVE_HEALTHY: "Healthy",
  ACTIVE_UNHEALTHY: "Unhealthy",
  UNHEALTHY: "Unhealthy",
  COMING_UP: "Starting",
  INIT_READ_REPLICA: "Starting",
  RESTORING: "Restoring",
  RESTARTING: "Restarting",
  PAUSING: "Pausing",
  PAUSED: "Paused",
  INACTIVE: "Inactive",
};

export function supabaseHealthLabel(status?: string, healthy?: boolean): string {
  if (status) {
    if (SUPABASE_HEALTH_LABELS[status]) return SUPABASE_HEALTH_LABELS[status];
    const words = status.replace(/_/g, " ").toLowerCase();
    return words.charAt(0).toUpperCase() + words.slice(1);
  }
  return healthy ? "Healthy" : "Unhealthy";
}

export const PROVIDER_LABELS: Record<KnowledgeProvider, string> = {
  postgres: "PostgreSQL",
  qdrant: "Qdrant",
  redis: "Redis",
  neo4j: "Neo4j",
  pinecone: "Pinecone",
  mysql: "MySQL",
  supabase: "Supabase",
};

export const PROVIDER_PORTS: Record<KnowledgeProvider, number | null> = {
  postgres: 5432,
  qdrant: 6333,
  redis: 6379,
  neo4j: 7687, // Bolt port — drivers use Bolt, not HTTP (7474)
  pinecone: null, // no port field
  mysql: 3306,
  supabase: 5432, // Supabase is PostgreSQL
};

// Only providers available for new store creation.
// Pinecone is SaaS-only (Cloud provider) so it's not offered in managed mode.
// MySQL is external-only for now — managed provisioning is not yet wired up.
// Qdrant is disabled for now — will be re-enabled later.
export const MANAGED_PROVIDERS: KnowledgeProvider[] = ["postgres", "redis", "neo4j"];
export const EXTERNAL_PROVIDERS: KnowledgeProvider[] = ["postgres", "supabase", "mysql", "redis", "neo4j", "pinecone"];

/** Which credential fields to show for each provider in the connect dialog. */
export const PROVIDER_FIELDS: Record<KnowledgeProvider, string[]> = {
  postgres: ["host", "port", "database", "username", "password"],
  mysql: ["host", "port", "database", "username", "password"],
  redis: ["host", "port", "password"],
  neo4j: ["host", "port", "username", "password"],
  qdrant: ["host", "port", "api_key"],
  pinecone: ["host", "api_key"],
  // Supabase connects as PostgreSQL; host/port/database/username are auto-filled
  // from the selected project, the user supplies only the database password.
  supabase: ["host", "port", "database", "username", "password"],
};

export function statusToColor(status: KnowledgeStatus): StatusBadgeColor {
  switch (status) {
    case "ready":
      return "success";
    case "provisioning":
    case "connecting":
    case "pending-acceptance":
      return "warning";
    case "error":
      return "error";
    default:
      return "muted";
  }
}

export function isTransitionalStatus(status: KnowledgeStatus): boolean {
  return ["provisioning", "connecting", "pending-acceptance"].includes(status);
}

export function statusLabel(status: KnowledgeStatus): string {
  switch (status) {
    case "provisioning":
      return "Provisioning";
    case "connecting":
      return "Connecting";
    case "pending-acceptance":
      return "Pending";
    case "ready":
      return "Ready";
    case "error":
      return "Error";
    default:
      return status;
  }
}

export function validateStoreName(name: string): string | null {
  if (!name) return null;
  if (name.length > 63) return "Name must be at most 63 characters";
  if (!/^[a-z0-9]/.test(name)) return "Name must start with a lowercase letter or digit";
  if (/^-|-$/.test(name)) return "Name must not start or end with a hyphen";
  if (/--/.test(name)) return "Name must not contain consecutive hyphens";
  if (!/^[a-z0-9-]+$/.test(name)) return "Name must contain only lowercase letters, digits, and hyphens";
  return null;
}

// ── Provider catalog (single source of truth for the client) ─────────────────

export const PROVIDER_CATEGORIES: Record<KnowledgeProvider, string> = {
  postgres: "Relational",
  qdrant: "Vector search",
  redis: "Key-value",
  neo4j: "Graph database",
  pinecone: "Vector search",
  mysql: "Relational",
  supabase: "Managed Postgres",
};

/** Providers available for new store creation. To enable a provider, add it here. */
export const ALL_PROVIDERS: KnowledgeProvider[] = ["postgres", "supabase", "mysql", "redis", "neo4j", "pinecone"];
export const MANAGED_SET = new Set<KnowledgeProvider>(MANAGED_PROVIDERS);

export const STORAGE_OPTIONS: { value: string; label: string }[] = [
  { value: "10Gi", label: "10 GB" },
  { value: "20Gi", label: "20 GB" },
  { value: "50Gi", label: "50 GB" },
  { value: "100Gi", label: "100 GB" },
];
