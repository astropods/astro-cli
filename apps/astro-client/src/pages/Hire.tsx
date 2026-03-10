import { useState, useMemo } from "react";
import type { Route } from "./+types/Hire";
import { Loader2 } from "lucide-react";
import { AgentCard } from "@/components/AgentCard";
import { CategorySidebar } from "@/components/browse/CategorySidebar";
import { SidebarLayout, SidebarBody } from "@/components/ui/sidebar-layout";
import { useAgents } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { getAgentCategories, getAgentDescription } from "@/lib/agent-utils";

const CATEGORIES = ["All", "Development", "Data", "Marketing", "Sales", "Support"];
export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const agentsData = await api.listAgents().catch(() => ({ agents: [], count: 0 }));

  return { agentsData };
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
  const categories = CATEGORIES;

  const filteredAgents = useMemo(() => {
    if (selectedCategory === "All") return agents;
    return agents.filter((agent) =>
      getAgentCategories(agent).includes(selectedCategory),
    );
  }, [agents, selectedCategory]);

  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-6 pb-6 pt-8 md:px-8 md:pb-8 md:pt-10 max-w-[1500px] mx-auto">
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
        <SidebarLayout>
          <CategorySidebar
            categories={categories}
            selected={selectedCategory}
            onSelect={setSelectedCategory}
          />
          <SidebarBody>
            <h1 className="text-heading-1 text-foreground">{selectedCategory === "All" ? "All Agents" : selectedCategory}</h1>
            <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3 content-start">
              {filteredAgents.map((agent) => (
                <AgentCard
                  key={`${agent.account}/${agent.name}`}
                  slug={`${agent.account}/${agent.name}`}
                  account={agent.account}
                  name={agent.name}
                  description={getAgentDescription(agent)}
                />
              ))}
            </div>
          </SidebarBody>
        </SidebarLayout>
      )}
    </div>
  );
}
