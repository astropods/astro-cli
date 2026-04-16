import type { KnowledgeProvider, KnowledgeStatus } from "@/lib/api";
import type { StatusBadgeColor } from "@/components/StatusBadge";

export const PROVIDER_LABELS: Record<KnowledgeProvider, string> = {
  postgres: "PostgreSQL",
  qdrant: "Qdrant",
  redis: "Redis",
  neo4j: "Neo4j",
  pinecone: "Pinecone",
  mysql: "MySQL",
};

export const PROVIDER_PORTS: Record<KnowledgeProvider, number | null> = {
  postgres: 5432,
  qdrant: 6333,
  redis: 6379,
  neo4j: 7474,
  pinecone: null, // no port field
  mysql: 3306,
};

export const MANAGED_PROVIDERS: KnowledgeProvider[] = ["postgres", "qdrant", "redis", "neo4j"];
export const EXTERNAL_PROVIDERS: KnowledgeProvider[] = ["postgres", "qdrant", "redis", "neo4j", "pinecone", "mysql"];

/** Which credential fields to show for each provider in the connect dialog. */
export const PROVIDER_FIELDS: Record<KnowledgeProvider, string[]> = {
  postgres: ["host", "port", "database", "username", "password"],
  mysql: ["host", "port", "database", "username", "password"],
  redis: ["host", "port", "password"],
  neo4j: ["host", "port", "username", "password"],
  qdrant: ["host", "port", "api_key"],
  pinecone: ["host", "api_key"],
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
      return "Pending Acceptance";
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
