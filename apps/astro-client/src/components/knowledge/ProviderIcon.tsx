import { CircleStackIcon } from "@heroicons/react/24/outline";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeProvider } from "@/lib/api";
import { useResolvedTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const PROVIDERS_WITH_ICON = new Set<KnowledgeProvider>(["postgres", "qdrant", "redis", "pinecone", "neo4j", "mysql"]);

export function ProviderIcon({ provider, className }: { provider: KnowledgeProvider; className?: string }) {
  const resolvedTheme = useResolvedTheme();
  if (provider === "supabase") {
    return (
      <svg
        viewBox="0 0 24 24"
        className={cn("text-[#3ECF8E] dark:text-[#3ECF8E]", className)}
        fill="currentColor"
        aria-hidden="true"
      >
        <path d="M11.9 1.036c-.015-.986-1.26-1.41-1.874-.637L.764 12.05C.33 12.587.736 13.36 1.424 13.36h9.736l.008 9.677c.015.987 1.261 1.41 1.875.637l9.262-11.65c.434-.537.028-1.31-.66-1.31h-9.74L11.9 1.036z" />
      </svg>
    );
  }
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
