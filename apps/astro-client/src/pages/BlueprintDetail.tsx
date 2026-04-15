import { useMemo, useState, useEffect, useRef } from "react";
import { useParams, Link, useNavigate } from "react-router";
import type { Route } from "./+types/BlueprintDetail";
import { Button } from "@/components/ui/button";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { LiveRevealConfetti } from "@/components/deployed-agent/detail/LiveRevealConfetti";
import {
  BlueprintDetailBreadcrumb,
  BlueprintDetailContent,
  BlueprintDetailSidebar,
  SidebarCard,
} from "@/components/blueprint-detail";
import { useBlueprint, useBlueprints } from "@/api/queries/blueprints";
import { useGitHubStatus } from "@/api/queries";
import { useAuth } from "@/lib/auth";
import { createServerApi } from "@/lib/api.server";
import {
  getBlueprintDescription,
  getBlueprintIntegrations,
  getBlueprintCategories,
  getBlueprintReadme,
  getBlueprintAuthors,
  getBlueprintCapabilities,
} from "@/lib/blueprint-utils";
import type { AccountPublic, Blueprint, BlueprintCardData } from "@/lib/api";

/** Minimal AGENT.md frontmatter parser — extracts description and body without a YAML dep. */
function parseAgentMD(content: string): BlueprintCardData | null {
  const match = content.match(/^---\n([\s\S]*?)\n---(?:\n|$)([\s\S]*)/);
  const card: BlueprintCardData = {};
  if (match) {
    const descMatch = match[1].match(/^description:\s*["']?(.*?)["']?\s*$/m);
    if (descMatch) card.description = descMatch[1].trim();
    const body = match[2].trim();
    if (body) card.body = body;
  } else if (content.trim()) {
    card.body = content.trim();
  }
  return card.description || card.body ? card : null;
}

// ─── Build success overlay ────────────────────────────────────────────────────

function BuildSuccessOverlay({ account, name, onDismiss }: { account: string; name: string; onDismiss: () => void }) {
  const containerRef = useRef<HTMLDivElement>(null);
  return (
    <div
      ref={containerRef}
      className="fixed inset-0 z-50 flex flex-col items-center justify-center overflow-hidden"
      style={{ background: "radial-gradient(ellipse at 50% 0%, hsla(40,50%,90%,0.95) 0%, hsla(40,30%,96%,0.98) 60%), hsl(40,20%,97%)" }}
    >
      <LiveRevealConfetti containerRef={containerRef} />
      <div className="flex flex-col items-center gap-6 text-center px-6">
        <div className="relative size-20 overflow-hidden rounded-2xl border border-border shadow-sm">
          <BlueprintIdentity account={account} name={name} size={80} className="size-full" />
          <div
            className="absolute left-0 right-0 h-[2px] opacity-80"
            style={{
              background: "linear-gradient(90deg, transparent, var(--color-teal-500), transparent)",
              animation: "scanLine 2.5s ease-in-out infinite",
              boxShadow: "0 0 12px 2px color-mix(in oklch, var(--color-teal-500) 30%, transparent)",
            }}
          />
        </div>
        <div>
          <div className="flex items-center justify-center gap-2 mb-2">
            <div className="flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground shrink-0">
              <svg viewBox="0 0 12 12" className="size-3" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M2.5 6l2.5 2.5 4.5-5" />
              </svg>
            </div>
            <h2 className="text-lg font-semibold">Blueprint is live</h2>
          </div>
          <p className="font-mono text-sm text-muted-foreground">{account}/{name}</p>
        </div>
        <Button onClick={onDismiss}>
          View blueprint →
        </Button>
      </div>
      <style>{`
        @keyframes scanLine {
          0%, 100% { top: 10%; }
          50% { top: 85%; }
        }
      `}</style>
    </div>
  );
}

// ─────────────────────────────────────────────────────────────────────────────

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";
  const origin = new URL(request.url).origin;

  const [blueprint, blueprintsData, accountData] = await Promise.all([
    account && agentSlug ? api.getBlueprint(account, agentSlug).catch(() => null) : null,
    api.listBlueprints().catch(() => ({ agents: [], count: 0 })),
    account ? api.getAccount(account).catch(() => null) : null,
  ]);

  // Batch-fetch accounts for recommended agent cards (profile pictures)
  // Seed with the already-fetched current account to avoid a duplicate request.
  const accountsMap: Record<string, AccountPublic> = {};
  if (accountData) accountsMap[accountData.name] = accountData;
  const uniqueAccounts = [...new Set(blueprintsData.agents.map((a) => a.account))]
    .filter((name) => !(name in accountsMap));
  const accountResults = await Promise.all(
    uniqueAccounts.map((name) => api.getAccount(name).catch(() => null)),
  );
  for (const acc of accountResults) {
    if (acc) accountsMap[acc.name] = acc;
  }

  const canonicalUrl = account && agentSlug ? `${origin}/${account}/${agentSlug}` : origin;
  const assetsBase = import.meta.env.VITE_ASSETS_URL?.replace(/\/$/, "");
  const avatarHandle = accountData?.name || account;
  const ogImage = assetsBase && avatarHandle
    ? `${assetsBase}/avatars/${encodeURIComponent(avatarHandle)}.jpg`
    : `${origin}/assets/placeholders/accounts/avatar_01.svg`;

  return { blueprint, blueprintsData, accountData, accountsMap, canonicalUrl, ogImage };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const blueprint = data?.blueprint;
  const canonicalUrl = data?.canonicalUrl;
  const ogImage = data?.ogImage;
  if (!blueprint) {
    return [
      { title: "Agent Details | Astro" },
      ...(canonicalUrl ? [{ property: "og:url", content: canonicalUrl } as const] : []),
      ...(ogImage ? [{ property: "og:image", content: ogImage } as const] : []),
    ];
  }
  const title = `${blueprint.account}/${blueprint.name} | Astro`;
  const description = blueprint.versions[0]?.agent_card?.description ?? `Check out ${blueprint.account}/${blueprint.name} on Astro.`;
  return [
    { title },
    { name: "description", content: description },
    { property: "og:type", content: "website" },
    ...(canonicalUrl ? [{ property: "og:url", content: canonicalUrl } as const] : []),
    { property: "og:title", content: title },
    { property: "og:description", content: description },
    ...(ogImage ? [{ property: "og:image", content: ogImage } as const] : []),
    { name: "twitter:card", content: "summary_large_image" },
    { name: "twitter:title", content: title },
    { name: "twitter:description", content: description },
    ...(ogImage ? [{ name: "twitter:image", content: ogImage } as const] : []),
  ];
};

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default function BlueprintDetail({ loaderData }: Route.ComponentProps) {
  const { account, agentSlug } = useParams<{ account?: string; agentSlug: string }>();
  const { isAuthenticated, accounts } = useAuth();
  const navigate = useNavigate();

  // Poll every 10s while draft so the page auto-updates once `ast push` completes.
  const { data: blueprint, isError, error } = useBlueprint(account ?? '', agentSlug ?? "", {
    initialData: loaderData?.blueprint ?? undefined,
    refetchInterval: (query) => (query.state.data?.versions.length === 0 ? 10_000 : false),
  });
  const { data: blueprintsData } = useBlueprints({
    initialData: loaderData?.blueprintsData ?? undefined,
  });

  const recommendedAgents = useMemo(() => {
    if (!blueprint || !blueprintsData) return [];
    const currentIntegrations = new Set(
      getBlueprintIntegrations(blueprint).map((i) => i.id.toLowerCase()),
    );
    const currentCategories = new Set(getBlueprintCategories(blueprint));
    return blueprintsData.agents
      .filter((a) => a.name !== agentSlug)
      .map((a) => {
        const ints = getBlueprintIntegrations(a);
        const cats = getBlueprintCategories(a);
        const score =
          ints.filter((i) => currentIntegrations.has(i.id.toLowerCase())).length +
          cats.filter((c) => currentCategories.has(c)).length;
        return { agent: a, score };
      })
      .sort((a, b) => b.score - a.score)
      .slice(0, 3)
      .map(({ agent: a }) => ({
        slug: `${a.account}/${a.name}`,
        account: a.account,
        name: a.name,
        description: getBlueprintDescription(a),
        deployCount: a.metrics?.deploy_count,
      }));
  }, [blueprint, blueprintsData, agentSlug]);

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Something went wrong</h1>
        <p className="text-muted-foreground text-sm mb-4">
          {error instanceof Error ? error.message : "Failed to load agent details."}
        </p>
        <Button asChild>
          <Link to="/blueprints">Blueprints</Link>
        </Button>
      </div>
    );
  }

  if (!blueprint) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/blueprints">Blueprints</Link>
        </Button>
      </div>
    );
  }

  const isDraft = blueprint.versions.length === 0;
  const canEdit = isAuthenticated && accounts.some((a) => a.name === blueprint.account);

  const { data: githubStatus, isLoading: githubStatusLoading } = useGitHubStatus(blueprint.account, blueprint.name, {
    enabled: isDraft && canEdit,
  });

  // Read the repo that was selected in the wizard — written to sessionStorage before navigating here.
  // Falls back to the server-confirmed value once useGitHubStatus resolves, then clears the entry.
  const sessionKey = `astro:github-repo:${blueprint.account}/${blueprint.name}`;
  const [sessionGithub, setSessionGithub] = useState<{ repo: string; branch: string; agent_md?: string; yml_found?: boolean } | undefined>(() => {
    try {
      const raw = sessionStorage.getItem(sessionKey);
      return raw ? JSON.parse(raw) : undefined;
    } catch { return undefined; }
  });
  useEffect(() => {
    if (githubStatus?.repo_full_name && sessionGithub) {
      // Remove from sessionStorage so it doesn't survive a hard refresh, but keep
      // the in-memory state alive — agent_md is still needed until the build completes.
      sessionStorage.removeItem(sessionKey);
    }
  }, [githubStatus?.repo_full_name, sessionGithub, sessionKey]);

  const githubRepoName = githubStatus?.repo_full_name ?? sessionGithub?.repo;
  const githubBranch = githubStatus?.branch ?? sessionGithub?.branch;

  // True when astropods.yml was found and a build exists (or is loading and we know yml was found).
  // Used to suppress the FINISH SETUP card — if a build is in flight the setup steps are irrelevant.
  const hasBuild = githubStatusLoading
    ? (sessionGithub?.yml_found ?? false)
    : (githubStatus?.builds?.length ?? 0) > 0;

  // Inject AGENT.md content as draft_card when the blueprint has no versions yet.
  const effectiveBlueprint: Blueprint = useMemo(() => {
    if (blueprint.versions.length > 0 || !sessionGithub?.agent_md) return blueprint;
    const card = parseAgentMD(sessionGithub.agent_md);
    if (!card) return blueprint;
    return { ...blueprint, draft_card: card };
  }, [blueprint, sessionGithub]);

  // Detect draft → published transition and show success overlay for the GitHub path.
  const [showBuildSuccess, setShowBuildSuccess] = useState(false);
  const wasDraftRef = useRef<boolean | null>(null);
  useEffect(() => {
    if (wasDraftRef.current === true && !isDraft && githubRepoName) {
      setShowBuildSuccess(true);
    }
    wasDraftRef.current = isDraft;
  }, [isDraft, githubRepoName]);


  const integrations = getBlueprintIntegrations(effectiveBlueprint);
  const categories = getBlueprintCategories(effectiveBlueprint);
  const readme = getBlueprintReadme(effectiveBlueprint);
  const authors = getBlueprintAuthors(effectiveBlueprint);
  const capabilities = getBlueprintCapabilities(effectiveBlueprint);

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-surface">
      {showBuildSuccess && (
        <BuildSuccessOverlay
          account={blueprint.account}
          name={blueprint.name}
          onDismiss={() => setShowBuildSuccess(false)}
        />
      )}
      <BlueprintDetailBreadcrumb account={blueprint.account} blueprintName={blueprint.name} hearted={blueprint.hearted} heartCount={blueprint.heart_count} />

      <div className="flex flex-1 overflow-y-auto">
      <div className="flex min-w-0 flex-1 max-w-[1200px] mx-auto">
        <BlueprintDetailContent
          account={blueprint.account}
          name={blueprint.name}
          categories={categories}
          canEdit={canEdit}
          readme={readme}
          isDraft={isDraft}
          onArchive={canEdit ? () => navigate(`/${blueprint.account}`) : undefined}
          hasBuild={hasBuild}
          githubRepoName={githubRepoName}
          visibility={blueprint.visibility}
          mobileSidebar={
            <SidebarCard
              agent={blueprint}
              integrations={integrations}
              capabilities={capabilities}
              authors={authors}
              installs={blueprint.metrics?.deploy_count}
              recommendedAgents={recommendedAgents}
              initialAccountData={loaderData?.accountData ?? undefined}
              canEdit={canEdit}
              githubRepoName={githubRepoName}
              githubBranch={githubBranch}
            />
          }
        />

        <BlueprintDetailSidebar
          agent={blueprint}
          integrations={integrations}
          capabilities={capabilities}
          authors={authors}
          installs={blueprint.metrics?.deploy_count}
          recommendedAgents={recommendedAgents}
          initialAccountData={loaderData?.accountData ?? undefined}
          canEdit={canEdit}
          githubRepoName={githubRepoName}
          githubBranch={githubBranch}
        />
      </div>
      </div>
    </div>
  );
}
