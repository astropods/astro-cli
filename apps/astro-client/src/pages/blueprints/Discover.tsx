import type { Route } from "./+types/Discover";
import { useAgents } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { AgentListView } from "@/components/browse/AgentListView";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const agentsData = await api.listAgents().catch(() => ({ agents: [], count: 0 }));
  return { agentsData };
}

export const meta: Route.MetaFunction = () => [
  { title: "Discover Blueprints | Astro" },
  { name: "description", content: "Discover public AI agent blueprints on Astro." },
  { property: "og:title", content: "Discover Blueprints | Astro" },
  { property: "og:description", content: "Discover public AI agent blueprints on Astro." },
];

export default function Discover({ loaderData }: Route.ComponentProps) {
  const { data, isLoading, isError, error, refetch } = useAgents({
    initialData: loaderData?.agentsData,
  });

  return (
    <>
      <h1 className="text-heading-1 text-foreground">Discover</h1>
      <AgentListView
        agents={data?.agents ?? []}
        isLoading={isLoading}
        isError={isError}
        error={error}
        refetch={refetch}
        emptyTitle="No blueprints available"
        emptyDescription="There are no blueprints in the registry yet."
      />
    </>
  );
}
