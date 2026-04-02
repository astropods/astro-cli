import { useParams, Link } from "react-router";
import type { Route } from "./+types/BlueprintDetail";
import { Button } from "@/components/ui/button";
import {
  BlueprintDetailBreadcrumb,
  BlueprintDetailContent,
  BlueprintDetailSidebar,
  BlueprintDetailSkeleton,
  SidebarCard,
} from "@/components/blueprint-detail";
import { useBlueprint, useBlueprints } from "@/api/queries/blueprints";
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
import type { AccountPublic } from "@/lib/api";

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
  const avatarVersion = accountData?.avatar_version;
  const ogImage = assetsBase && avatarHandle
    ? `${assetsBase}/avatars/${encodeURIComponent(avatarHandle)}.jpg${avatarVersion ? `?v=${avatarVersion}` : ""}`
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

  // Support both /:account/:agentSlug and legacy /:agentSlug routes
  const { data: blueprint, isLoading, isError, error } = useBlueprint(account ?? '', agentSlug ?? "", {
    initialData: loaderData?.blueprint ?? undefined,
  });
  const { data: blueprintsData } = useBlueprints({
    initialData: loaderData?.blueprintsData ?? undefined,
  });
  if (isLoading) {
    return <BlueprintDetailSkeleton />;
  }

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

  const recommendedAgents = (() => {
    if (!blueprint || !blueprintsData) return [];
    const currentIntegrations = new Set(
      getBlueprintIntegrations(blueprint).map((integration) => integration.id.toLowerCase()),
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
        deployCount: a.metrics?.deploy_count,
      }));
  })();

  const { isAuthenticated, accounts } = useAuth();
  const canEdit = isAuthenticated && accounts.some((a) => a.name === blueprint.account);

  const integrations = getBlueprintIntegrations(blueprint);
  const categories = getBlueprintCategories(blueprint);
  const readme = getBlueprintReadme(blueprint);
  const authors = getBlueprintAuthors(blueprint);
  const capabilities = getBlueprintCapabilities(blueprint);

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-surface">
      <BlueprintDetailBreadcrumb account={blueprint.account} blueprintName={blueprint.name} hearted={blueprint.hearted} heartCount={blueprint.heart_count} />

      <div className="flex flex-1 overflow-y-auto">
      <div className="flex min-w-0 flex-1 max-w-[1200px] mx-auto">
        <BlueprintDetailContent
          account={blueprint.account}
          name={blueprint.name}
          visibility={blueprint.visibility}
          categories={categories}
          avatarUrl={blueprint.avatar_url}
          canEdit={canEdit}
          onArchive={canEdit ? () => {} : undefined}
          readme={readme}
          mobileSidebar={
            <SidebarCard
              agent={blueprint}
              integrations={integrations}
              capabilities={capabilities}
              authors={authors}
              installs={blueprint.metrics?.deploy_count}
              recommendedAgents={recommendedAgents}
              initialAccountData={loaderData?.accountData ?? undefined}
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
        />
      </div>
      </div>
    </div>
  );
}
