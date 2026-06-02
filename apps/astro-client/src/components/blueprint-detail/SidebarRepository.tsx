import { ExternalLink } from "lucide-react";
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

// Build a URL that points into a subdirectory inside the repo for the known
// providers. `HEAD` resolves to the default branch on github / gitlab /
// bitbucket without us having to know whether it's `main`, `master`, etc.
// Unknown providers fall back to the repo root URL — better that than fabricate
// a path the host doesn't understand.
export function buildRepoDirectoryUrl(url: string, directory?: string): string {
  if (!directory) return url;
  try {
    const parsed = new URL(url);
    const hostname = parsed.hostname.replace(/^www\./, "");
    const base = parsed.toString().replace(/\/$/, "").replace(/\.git$/, "");
    const cleanDir = directory.replace(/^\/+/, "").replace(/\/+$/, "");
    switch (hostname) {
      case "github.com":
      case "gist.github.com":
        return `${base}/tree/HEAD/${cleanDir}`;
      case "gitlab.com":
        return `${base}/-/tree/HEAD/${cleanDir}`;
      case "bitbucket.org":
        return `${base}/src/HEAD/${cleanDir}`;
      default:
        return url;
    }
  } catch {
    return url;
  }
}

export function SidebarRepository({ repository }: SidebarRepositoryProps) {
  const integrationId = getProviderIntegrationId(repository.url);
  const label = getDisplayLabel(repository.url);

  return (
    <SidebarSection title="Repository">
      <div className="flex flex-col gap-0.5 min-w-0">
        <a
          href={repository.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1.5 text-[13px] text-foreground hover:underline underline-offset-4 transition-colors min-w-0"
        >
          {integrationId && (
            <span className="flex h-4 w-4 shrink-0 items-center justify-center">
              <img
                src={getIntegrationIconUrl(integrationId, "light")}
                alt=""
                className="h-full w-full object-contain dark:hidden"
                loading="lazy"
              />
              <img
                src={getIntegrationIconUrl(integrationId, "dark")}
                alt=""
                className="hidden h-full w-full object-contain dark:block"
                loading="lazy"
              />
            </span>
          )}
          <span className="truncate">{label}</span>
          <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
        </a>
        {repository.directory && (
          // Indent the path so the └─ connector sits under the repo icon column
          // (icon 16px + gap 6px ≈ 22px), giving the visual sense that the path
          // is a child of the repo row above.
          <a
            href={buildRepoDirectoryUrl(repository.url, repository.directory)}
            target="_blank"
            rel="noopener noreferrer"
            className="group flex items-center gap-1 pl-[22px] text-[11px] text-muted-foreground font-mono transition-colors min-w-0"
          >
            <span aria-hidden className="shrink-0">└─</span>
            <span className="truncate pb-0.5 group-hover:underline underline-offset-2">/{repository.directory}</span>
          </a>
        )}
      </div>
    </SidebarSection>
  );
}
