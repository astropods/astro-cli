import { useState, useMemo } from "react";
import { Link } from "react-router";
import type { Route } from "./+types/Hire";
import { PaperAirplaneIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { PageTitle } from "@/components/PageTitle";
import { Button } from "@/components/ui/button";
import { BrowseAgentCard } from "@/components/browse/BrowseAgentCard";
import { CategorySidebar } from "@/components/browse/CategorySidebar";
import { PublishCTA } from "@/components/browse/PublishCTA";
import { useAgents } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { getAgentCategories, getAgentDescription } from "@/lib/agent-utils";
import type { AccountPublic } from "@/lib/api";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const agentsData = await api.listAgents().catch(() => ({ agents: [], count: 0 }));

  // Batch-fetch all unique accounts in parallel to get owner profile pictures
  const uniqueAccounts = [...new Set(agentsData.agents.map((a) => a.account))];
  const accountResults = await Promise.all(
    uniqueAccounts.map((name) => api.getAccount(name).catch(() => null)),
  );
  const accountsMap: Record<string, AccountPublic> = {};
  for (const acc of accountResults) {
    if (acc) accountsMap[acc.name] = acc;
  }

  return { agentsData, accountsMap };
}

export const meta: Route.MetaFunction = () => [
  { title: "Browse Agents | Astro" },
  { name: "description", content: "Browse AI agents available for hire on Astro. Find agents for engineering, operations, support, and more." },
  { property: "og:title", content: "Browse Agents | Astro" },
  { property: "og:description", content: "Browse AI agents available for hire on Astro." },
];

export default function Hire({ loaderData }: Route.ComponentProps) {
  const [selectedCategory, setSelectedCategory] = useState("All");

  const { data, isLoading, isError, error, refetch } = useAgents({
    initialData: loaderData?.agentsData,
  });
  const agents = data?.agents ?? [];
  const accountsMap = loaderData?.accountsMap ?? {};

  const categories = useMemo(() => {
    const tagSet = new Set<string>(["Developer Tools", "Getting Started", "Security", "Starter"]);
    for (const agent of agents) {
      for (const tag of getAgentCategories(agent)) {
        tagSet.add(tag);
      }
    }
    return ["All", ...Array.from(tagSet).sort()];
  }, [agents]);

  const filteredAgents = useMemo(() => {
    if (selectedCategory === "All") return agents;
    return agents.filter((agent) =>
      getAgentCategories(agent).includes(selectedCategory),
    );
  }, [agents, selectedCategory]);

  return (
    <div className="@container w-full flex-1 overflow-y-auto px-6 pb-6 pt-4 md:px-8 md:pb-8 md:pt-6 max-w-[1500px] mx-auto">
      {isLoading ? (
        <div role="status" aria-label="Loading agents" className="flex items-center justify-center py-12">
          <Loader2 size={32} className="animate-spin text-muted-foreground" />
        </div>
      ) : isError ? (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-700">
          <p className="font-medium">Failed to load agents</p>
          <p className="text-sm">
            {(error as { error_description?: string })?.error_description ??
              (error instanceof Error ? error.message : "An unexpected error occurred")}
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-2 cursor-pointer rounded-md border border-red-300 bg-white px-3 py-1 text-sm text-red-700 hover:bg-red-50"
          >
            Retry
          </button>
        </div>
      ) : agents.length === 0 ? (
        <div className="rounded-lg border border-border p-8 text-center">
          <h3 className="mb-2 text-lg font-medium">No agents available</h3>
          <p className="text-sm text-muted-foreground">
            There are no agents in the registry yet.
          </p>
        </div>
      ) : (
        <div className="flex flex-col gap-6 md:grid md:grid-cols-[9rem_1fr] md:gap-x-6 md:gap-y-6">
          <PageTitle
            title="Browse Agents"
            actions={
              <Button asChild className="hidden @[540px]:inline-flex">
                <Link to="/request-agent">
                  <PaperAirplaneIcon className="size-4" />
                  Request agent
                </Link>
              </Button>
            }
            className="md:col-start-2"
          />
          <CategorySidebar
            categories={categories}
            selected={selectedCategory}
            onSelect={setSelectedCategory}
          />
          <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3 content-start">
            {filteredAgents.map((agent) => (
              <BrowseAgentCard
                key={`${agent.account}/${agent.name}`}
                slug={`${agent.account}/${agent.name}`}
                account={agent.account}
                name={agent.name}
                description={getAgentDescription(agent)}
                categories={getAgentCategories(agent)}
                ownerPictureUrl={accountsMap[agent.account]?.owner?.profile_picture_url}
              />
            ))}
            <PublishCTA />
          </div>
        </div>
      )}
    </div>
  );
}
