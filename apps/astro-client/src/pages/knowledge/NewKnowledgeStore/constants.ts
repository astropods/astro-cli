import type { KnowledgeProvider } from "@/lib/api";
import { MANAGED_PROVIDERS } from "@/components/knowledge/knowledge-utils";

export const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);

export const PROVIDER_CATEGORIES: Record<KnowledgeProvider, string> = {
  postgres: "Relational",
  qdrant: "Vector search",
  redis: "Key-value",
  neo4j: "Graph database",
  pinecone: "Vector search",
  mysql: "Relational",
};

export const ALL_PROVIDERS: KnowledgeProvider[] = ["postgres", "qdrant", "redis", "neo4j", "mysql", "pinecone"];
export const MANAGED_SET = new Set<KnowledgeProvider>(MANAGED_PROVIDERS);

export const STORAGE_OPTIONS: { value: string; label: string }[] = [
  { value: "10Gi", label: "10 GB" },
  { value: "20Gi", label: "20 GB" },
  { value: "50Gi", label: "50 GB" },
  { value: "100Gi", label: "100 GB" },
  { value: "1Ti", label: "1 TB" },
];
