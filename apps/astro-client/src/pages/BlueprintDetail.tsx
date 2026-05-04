import { useMemo, useState, useEffect, useRef } from "react";
import { Check } from "lucide-react";
import { useParams, Link, useNavigate } from "react-router";
import type { BlueprintCardProps } from "@/components/BlueprintCard";
import type { Route } from "./+types/BlueprintDetail";
import { Button } from "@/components/ui/button";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { LiveRevealConfetti } from "@/components/ui/LiveRevealConfetti";
import { GradientGridWash } from "@/components/GradientGridWash";
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
import type { AccountPublic, Blueprint } from "@/lib/api";

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
              background: "linear-gradient(90deg, transparent, var(--color-slate-500), transparent)",
              animation: "scanLine 2.5s ease-in-out infinite",
              boxShadow: "0 0 12px 2px color-mix(in oklch, var(--color-slate-500) 30%, transparent)",
            }}
          />
        </div>
        <div>
          <div className="flex items-center justify-center gap-2 mb-2">
            <div className="flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground shrink-0">
              <Check className="size-3" />
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
  const origin = import.meta.env.VITE_FRONTEND_URL || new URL(request.url).origin;

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
  const isPublic = blueprint?.visibility === "public"
    && (blueprint?.versions.length ?? 0) > 0;
  const ogImage = isPublic && account && agentSlug
    ? `${origin}/badge/agents/${encodeURIComponent(account)}/${encodeURIComponent(agentSlug)}.png`
    : null;

  // Pre-fetch GitHub status for draft blueprints owned by the authenticated user so
  // AGENT.md is available immediately on page load (no flash, no sessionStorage).
  const githubStatus = blueprint?.versions.length === 0
    ? await api.getGitHubStatus(account, agentSlug).catch(() => null)
    : null;

  return { blueprint, blueprintsData, accountData, accountsMap, canonicalUrl, ogImage, isPublic, githubStatus };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const blueprint = data?.blueprint;
  const canonicalUrl = data?.canonicalUrl;
  const ogImage = data?.ogImage; // null for private blueprints
  if (!blueprint) {
    return [
      { title: "Agent Details | Astro" },
      ...(canonicalUrl ? [{ property: "og:url", content: canonicalUrl } as const] : []),
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
    // OG image only for public blueprints — private blueprints get no image unfurl
    ...(ogImage
      ? [
          { property: "og:image", content: ogImage } as const,
          { property: "og:image:width", content: "1200" } as const,
          { property: "og:image:height", content: "628" } as const,
          { name: "twitter:card", content: "summary_large_image" } as const,
          { name: "twitter:image", content: ogImage } as const,
        ]
      : [{ name: "twitter:card", content: "summary" } as const]),
    { name: "twitter:title", content: title },
    { name: "twitter:description", content: description },
  ];
};

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default function BlueprintDetail({ loaderData }: Route.ComponentProps) {
  const { account, agentSlug } = useParams<{ account?: string; agentSlug: string }>();
  const { isAuthenticated, accounts } = useAuth();

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
        avatarColors: a.avatar_colors,
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

  const canEdit = isAuthenticated && accounts.some((a) => a.name === blueprint.account);

  return (
    <BlueprintDetailInner
      blueprint={blueprint}
      canEdit={canEdit}
      loaderData={loaderData}
      recommendedAgents={recommendedAgents}
    />
  );
}

function BlueprintDetailInner({
  blueprint,
  canEdit,
  loaderData,
  recommendedAgents,
}: {
  blueprint: Blueprint;
  canEdit: boolean;
  loaderData: Route.ComponentProps['loaderData'];
  recommendedAgents: BlueprintCardProps[];
}) {
  const navigate = useNavigate();
  const isDraft = blueprint.versions.length === 0;

  const { data: githubStatus } = useGitHubStatus(blueprint.account, blueprint.name, {
    enabled: isDraft && canEdit,
    initialData: loaderData?.githubStatus ?? undefined,
  });

  const githubRepoName = githubStatus?.repo_full_name;
  const githubBranch = githubStatus?.branch;

  // True once status is loaded; optimistically true before first response to suppress flash.
  const hasBuild = githubStatus === undefined
    ? true
    : (githubStatus.builds?.length ?? 0) > 0;

  // Latch draft_card once seen — survives status refetches and rebuilds.
  const [latchedDraftCard, setLatchedDraftCard] = useState(loaderData?.githubStatus?.draft_card);
  useEffect(() => {
    if (githubStatus?.draft_card && !latchedDraftCard) {
      setLatchedDraftCard(githubStatus.draft_card);
    }
  }, [githubStatus?.draft_card, latchedDraftCard]);

  // Inject draft_card into the blueprint when no versions exist yet.
  const effectiveBlueprint: Blueprint = useMemo(() => {
    if (blueprint.versions.length > 0 || !latchedDraftCard) return blueprint;
    return { ...blueprint, draft_card: latchedDraftCard };
  }, [blueprint, latchedDraftCard]);

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
    <div className="flex flex-col flex-1 min-h-0 bg-background">
      {showBuildSuccess && (
        <BuildSuccessOverlay
          account={blueprint.account}
          name={blueprint.name}
          onDismiss={() => setShowBuildSuccess(false)}
        />
      )}
      <BlueprintDetailBreadcrumb account={blueprint.account} blueprintName={blueprint.name} hearted={blueprint.hearted} heartCount={blueprint.heart_count} shareUrl={loaderData?.canonicalUrl} />

      <div className="relative flex flex-1 overflow-y-auto">
        <GradientGridWash colors={effectiveBlueprint.avatar_colors} />

      <div className="relative flex min-w-0 flex-1 max-w-[1200px] mx-auto">
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
          mobileSidebar={
            <SidebarCard
              agent={effectiveBlueprint}
              integrations={integrations}
              capabilities={capabilities}
              authors={authors}
              publishers={blueprint.publishers}
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
          agent={effectiveBlueprint}
          integrations={integrations}
          capabilities={capabilities}
          authors={authors}
          publishers={blueprint.publishers}
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
