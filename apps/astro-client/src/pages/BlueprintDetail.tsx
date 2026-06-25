import { useMemo, useState, useEffect } from "react";
import { useParams, Link, useNavigate } from "react-router";
import type { BlueprintCardProps } from "@/components/BlueprintCard";
import type { Route } from "./+types/BlueprintDetail";
import { Button } from "@/components/ui/button";
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
        avatarUrl: a.avatar_url,
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

  const integrations = getBlueprintIntegrations(effectiveBlueprint);
  const categories = getBlueprintCategories(effectiveBlueprint);
  const readme = getBlueprintReadme(effectiveBlueprint);
  const authors = getBlueprintAuthors(effectiveBlueprint);
  const capabilities = getBlueprintCapabilities(effectiveBlueprint);
  return (
    <div className="flex flex-col flex-1 min-h-0 bg-background">
      <BlueprintDetailBreadcrumb account={blueprint.account} blueprintName={blueprint.name} hearted={blueprint.hearted} heartCount={blueprint.heart_count} shareUrl={loaderData?.canonicalUrl} />

      <div className="relative flex flex-1 overflow-y-auto">
        <GradientGridWash colors={effectiveBlueprint.avatar_colors} darkGridOpacity={0.2} />

      <div className="relative flex min-w-0 flex-1 max-w-[1200px] mx-auto">
        <BlueprintDetailContent
          account={blueprint.account}
          name={blueprint.name}
          categories={categories}
          canEdit={canEdit}
          readme={readme}
          avatarUrl={blueprint.avatar_url}

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
