import { useParams, Link } from "react-router";
import type { Route } from "./+types/AgentDetail";
import { Button } from "@/components/ui/button";
import {
  AgentDetailBreadcrumb,
  AgentDetailContent,
  AgentDetailSidebar,
  AgentDetailSkeleton,
  SidebarCard,
} from "@/components/agent-detail";
import { useAgent, useAgents } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import {
  getAgentDescription,
  getAgentIntegrations,
  getAgentCategories,
  getAgentReadme,
  getAgentAuthors,
  getAgentCapabilities,
} from "@/lib/agent-utils";
import type { AccountPublic } from "@/lib/api";

export async function loader({ params, request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const account = params.account ?? "";
  const agentSlug = params.agentSlug ?? "";

  const [agent, agentsData, accountData] = await Promise.all([
    account && agentSlug ? api.getAgent(account, agentSlug).catch(() => null) : null,
    api.listAgents().catch(() => ({ agents: [], count: 0 })),
    account ? api.getAccount(account).catch(() => null) : null,
  ]);

  // Batch-fetch accounts for recommended agent cards (profile pictures)
  // Seed with the already-fetched current account to avoid a duplicate request.
  const accountsMap: Record<string, AccountPublic> = {};
  if (accountData) accountsMap[accountData.name] = accountData;
  const uniqueAccounts = [...new Set(agentsData.agents.map((a) => a.account))]
    .filter((name) => !(name in accountsMap));
  const accountResults = await Promise.all(
    uniqueAccounts.map((name) => api.getAccount(name).catch(() => null)),
  );
  for (const acc of accountResults) {
    if (acc) accountsMap[acc.name] = acc;
  }

  return { agent, agentsData, accountData, accountsMap };
}

export const meta: Route.MetaFunction = ({ data }) => {
  const agent = data?.agent;
  if (!agent) {
    return [{ title: "Agent Details | Astro" }];
  }
  const description = agent.versions[0]?.agent_card?.description ?? "";
  return [
    { title: `${agent.account}/${agent.name} | Astro` },
    { name: "description", content: description },
    { property: "og:title", content: `${agent.account}/${agent.name} | Astro` },
    { property: "og:description", content: description },
  ];
};

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
  if (isLoading) {
    return <AgentDetailSkeleton />;
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Something went wrong</h1>
        <p className="text-muted-foreground text-sm mb-4">
          {error instanceof Error ? error.message : "Failed to load agent details."}
        </p>
        <Button asChild>
          <Link to="/browse">Browse Agents</Link>
        </Button>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex flex-col items-center justify-center py-16 px-6">
        <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
        <p className="text-muted-foreground text-sm mb-4">
          The agent you're looking for doesn't exist or has been removed.
        </p>
        <Button asChild>
          <Link to="/browse">Browse Agents</Link>
        </Button>
      </div>
    );
  }

  const recommendedAgents = (() => {
    if (!agent || !agentsData) return [];
    const currentIntegrations = new Set(
      getAgentIntegrations(agent).map((integration) => integration.id.toLowerCase()),
    );
    const currentCategories = new Set(getAgentCategories(agent));
    return agentsData.agents
      .filter((a) => a.name !== agentSlug)
      .map((a) => {
        const ints = getAgentIntegrations(a);
        const cats = getAgentCategories(a);
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
        description: getAgentDescription(a),
      }));
  })();

  const integrations = getAgentIntegrations(agent);
  const categories = getAgentCategories(agent);
  const readme = getAgentReadme(agent);
  const authors = getAgentAuthors(agent);
  const capabilities = getAgentCapabilities(agent);

  return (
    <div className="flex flex-col flex-1 min-h-0 bg-surface">
      <AgentDetailBreadcrumb account={agent.account} agentName={agent.name} hearted={agent.hearted} heartCount={agent.heart_count} />

      <div className="flex flex-1 overflow-y-auto">
      <div className="flex min-w-0 flex-1 max-w-[1200px] mx-auto">
        <AgentDetailContent
          account={agent.account}
          name={agent.name}
          visibility={agent.visibility}
          categories={categories}
          readme={readme}
          mobileSidebar={
            <SidebarCard
              agent={agent}
              integrations={integrations}
              capabilities={capabilities}
              authors={authors}
              recommendedAgents={recommendedAgents}
              initialAccountData={loaderData?.accountData ?? undefined}
            />
          }
        />

        <AgentDetailSidebar
          agent={agent}
          integrations={integrations}
          capabilities={capabilities}
          authors={authors}
          recommendedAgents={recommendedAgents}
          initialAccountData={loaderData?.accountData ?? undefined}
        />
      </div>
      </div>
    </div>
  );
}
