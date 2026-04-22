import { CircleStackIcon } from "@heroicons/react/24/outline";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { KnowledgeProvider } from "@/lib/api";
import { cn } from "@/lib/utils";
import { PROVIDERS_WITH_ICON } from "./constants";

export function ProviderIcon({ provider, className }: { provider: KnowledgeProvider; className?: string }) {
  if (PROVIDERS_WITH_ICON.has(provider)) {
    return (
      <img
        src={getIntegrationIconUrl(provider, "light")}
        alt=""
        className={cn("object-contain", className)}
        loading="lazy"
      />
    );
  }
  return <CircleStackIcon className={cn("text-muted-foreground/60", className)} />;
}
