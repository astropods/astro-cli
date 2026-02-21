import { useParams, Link } from "react-router";
import type { Route } from "./+types/AgentDetail";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AgentDetailBreadcrumb,
  AgentDetailContent,
  AgentDetailSidebar,
} from "@/components/agent-detail";
import { useAgent, useAgents } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import {
  getLatestSpec,
  getAgentDescription,
  getAgentIntegrations,
  getAgentCategories,
} from "@/lib/agent-utils";

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";

  const [agent, agentsData, accountData] = await Promise.all([
    account && agentSlug ? api.getAgent(account, agentSlug).catch(() => null) : null,
    api.listAgents().catch(() => ({ agents: [], count: 0 })),
    account ? api.getAccount(account).catch(() => null) : null,
  ]);

  return { agent, agentsData, accountData };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const agent = data?.agent;
  if (!agent) {
    return [{ title: "Agent Details | Astro" }];
  }
  const description = agent.versions[0]?.spec?.meta?.description ?? "";
  return [
    { title: `${agent.account}/${agent.name} | Astro` },
    { name: "description", content: description },
    { property: "og:title", content: `${agent.account}/${agent.name} | Astro` },
    { property: "og:description", content: description },
  ];
};

// ---------------------------------------------------------------------------
// Loading skeleton
// ---------------------------------------------------------------------------

function AgentDetailSkeleton() {
  return (
    <div className="flex flex-col flex-1 min-h-0">
      {/* Breadcrumb skeleton */}
      <div className="flex items-center justify-between px-6 py-3 border-b border-border">
        <div className="flex items-center gap-2">
          <Skeleton className="h-4 w-24" />
          <Skeleton className="h-3.5 w-3.5" />
          <Skeleton className="h-4 w-40" />
        </div>
        <div className="flex gap-1">
          <Skeleton className="size-7 rounded" />
          <Skeleton className="size-7 rounded" />
        </div>
      </div>

      {/* Content skeleton */}
      <div className="flex flex-1 overflow-y-auto">
        {/* Left column */}
        <div className="flex-1 min-w-0 p-6 md:p-8 max-w-3xl">
          <Skeleton className="h-7 w-64 mb-3" />
          <div className="space-y-2 mb-5">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </div>
          <div className="flex gap-2 mb-8">
            <Skeleton className="h-6 w-20 rounded-full" />
            <Skeleton className="h-6 w-16 rounded-full" />
          </div>
          <div className="mb-8">
            <Skeleton className="h-3 w-16 mb-3" />
            <Skeleton className="h-40 w-full rounded-lg" />
          </div>
          <div className="mb-8">
            <Skeleton className="h-3 w-32 mb-3" />
            <div className="space-y-2">
              <Skeleton className="h-4 w-56" />
              <Skeleton className="h-4 w-48" />
            </div>
          </div>
        </div>

        {/* Right sidebar skeleton */}
        <div className="hidden lg:block w-[340px] shrink-0 p-6">
          <div className="rounded-lg border border-border p-5 space-y-5">
            <Skeleton className="h-10 w-full rounded" />
            <div className="h-px bg-border" />
            <div className="flex items-center gap-3">
              <Skeleton className="size-10 rounded-full" />
              <Skeleton className="h-4 w-24" />
            </div>
            <div className="h-px bg-border" />
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Skeleton className="h-3 w-12" />
                <Skeleton className="h-4 w-16" />
              </div>
              <div className="space-y-1">
                <Skeleton className="h-3 w-12" />
                <Skeleton className="h-4 w-16" />
              </div>
            </div>
            <div className="h-px bg-border" />
            <div className="space-y-2">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-10 w-full rounded-lg" />
              <Skeleton className="h-10 w-full rounded-lg" />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default function AgentDetail({ loaderData }: Route.ComponentProps) {
  const { account, agentSlug } = useParams<{ account?: string; agentSlug: string }>();

  // Support both /:account/:agentSlug and legacy /:agentSlug routes
  const { data: agent, isLoading, isError, error } = useAgent(account ?? '', agentSlug ?? "", {
    initialData: loaderData?.agent ?? undefined,
  });
  const { data: agentsData } = useAgents({
    initialData: loaderData?.agentsData ?? undefined,
  });

  const recommendedAgents = (() => {
    if (!agent || !agentsData) return [];
    const currentIntegrations = new Set(getAgentIntegrations(agent));
    const currentCategories = new Set(getAgentCategories(agent));
    return agentsData.agents
      .filter((a) => a.name !== agentSlug)
      .map((a) => {
        const ints = getAgentIntegrations(a);
        const cats = getAgentCategories(a);
        const score =
          ints.filter((i) => currentIntegrations.has(i)).length +
          cats.filter((c) => currentCategories.has(c)).length;
        return { agent: a, integrations: ints, categories: cats, score };
      })
      .sort((a, b) => b.score - a.score)
      .slice(0, 2)
      .map(({ agent: a, integrations, categories }) => ({
        slug: `${a.account}/${a.name}`,
        account: a.account,
        name: a.name,
        description: getAgentDescription(a),
        integrations,
        categories,
      }));
  })();

  if (isLoading) {
    return <AgentDetailSkeleton />;
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Something went wrong</h1>
        <p className="text-stone-500 text-sm mb-4">
          {error instanceof Error ? error.message : "Failed to load agent details."}
        </p>
        <Button asChild>
          <Link to="/hire">Browse Agents</Link>
        </Button>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
        <p className="text-stone-500 text-sm mb-4">
          The agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/hire">Browse Agents</Link>
        </Button>
      </div>
    );
  }

  const description = getAgentDescription(agent);
  const integrations = getAgentIntegrations(agent);
  const spec = getLatestSpec(agent);
  const readme = agent.versions[0]?.readme;
  const credentials = spec?.integrations
    ? Object.values(spec.integrations)
    : [];
  const safetyPermissions = credentials.map(
    (i) => `Access to ${i.provider}`,
  );

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-white">
      <AgentDetailBreadcrumb account={agent.account} agentName={agent.name} />

      <div className="flex flex-1 overflow-y-auto">
      <div className="flex flex-1 max-w-[1200px] mx-auto">
        <AgentDetailContent
          account={agent.account}
          name={agent.name}
          description={description}
          categories={getAgentCategories(agent)}
          readme={readme}
          safetyPermissions={safetyPermissions}
          recommendedAgents={recommendedAgents}
        />

        <AgentDetailSidebar
          agent={agent}
          integrations={integrations}
          permissions={safetyPermissions}
          initialAccountData={loaderData?.accountData ?? undefined}
        />
      </div>
      </div>
    </div>
  );
}
