import { CircleStackIcon } from "@heroicons/react/24/outline";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeProvider } from "@/lib/api";
import { useResolvedTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);

export function ProviderIcon({ provider, className }: { provider: KnowledgeProvider; className?: string }) {
  const resolvedTheme = useResolvedTheme();
  if (PROVIDERS_WITH_ICON.has(provider)) {
    return (
      <img
        src={getIntegrationIconUrl(provider, resolvedTheme)}
        alt=""
        className={cn("object-contain", className)}
        loading="lazy"
      />
    );
  }
  return <CircleStackIcon className={cn("text-muted-foreground/60", className)} />;
}
