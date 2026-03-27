import { SidebarSection } from "./SidebarSection";
import { getIntegrationIconUrl } from "@/lib/assets";
import type { BlueprintCardRepo } from "@/lib/api";

export interface SidebarRepositoryProps {
  repository: BlueprintCardRepo;
}

const providerIntegrationIds: Record<string, string> = {
  "github.com": "github",
  "gist.github.com": "github",
  "gitlab.com": "gitlab",
  "bitbucket.org": "bitbucket",
};

function getProviderIntegrationId(url: string): string | null {
  try {
    const hostname = new URL(url).hostname.replace(/^www\./, "");
    return providerIntegrationIds[hostname] ?? null;
  } catch {
    return null;
  }
}

function getDisplayLabel(url: string): string {
  try {
    const parsed = new URL(url);
    const hostname = parsed.hostname.replace(/^www\./, "");
    const path = parsed.pathname.replace(/^\//, "").replace(/\.git$/, "");
    if (
      hostname === "github.com" ||
      hostname === "gitlab.com" ||
      hostname === "bitbucket.org"
    ) {
      return path;
    }
    return `${hostname}/${path}`;
  } catch {
    return url;
  }
}

export function SidebarRepository({ repository }: SidebarRepositoryProps) {
  const integrationId = getProviderIntegrationId(repository.url);
  const label = getDisplayLabel(repository.url);

  return (
    <SidebarSection title="Repository">
      <a
        href={repository.url}
        target="_blank"
        rel="noopener noreferrer"
        className="flex items-center gap-2.5 text-[13px] text-foreground hover:underline hover:decoration-primary underline-offset-4 transition-colors min-w-0"
      >
        {integrationId && (
          <span className="flex h-4 w-4 shrink-0 items-center justify-center">
            <img
              src={getIntegrationIconUrl(integrationId, "light")}
              alt=""
              className="h-full w-full object-contain"
              loading="lazy"
            />
          </span>
        )}
        <span className="truncate">{label}</span>
        {repository.directory && (
          <span className="truncate text-[11px] text-muted-foreground font-mono">
            /{repository.directory}
          </span>
        )}
      </a>
    </SidebarSection>
  );
}
