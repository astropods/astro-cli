import type { Route } from "./+types/Discover";
import { useBlueprints } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { useAuth } from "@/lib/auth";

export async function loader({ request }: Route.LoaderArgs) {
  const api = createServerApi(request);
  const blueprintsData = await api.listBlueprints().catch(() => ({ agents: [], count: 0 }));
  return { blueprintsData };
}

export const meta: Route.MetaFunction = () => [
  { title: "Discover Blueprints | Astro" },
  { name: "description", content: "Discover public AI agent blueprints on Astro." },
  { property: "og:title", content: "Discover Blueprints | Astro" },
  { property: "og:description", content: "Discover public AI agent blueprints on Astro." },
];

export default function Discover({ loaderData }: Route.ComponentProps) {
  const { data, isLoading, isError, error, refetch } = useBlueprints({
    initialData: loaderData?.blueprintsData,
  });
  const { accounts } = useAuth();
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  return (
    <>
      <h1 className="text-heading-1 text-foreground">Discover blueprints</h1>
      <BlueprintListView
        blueprints={data?.agents ?? []}
        isLoading={isLoading}
        isError={isError}
        error={error}
        refetch={refetch}
        emptyTitle="No blueprints available"
        emptyDescription="There are no blueprints in the registry yet."
        ownerAccounts={ownerAccounts}
      />
    </>
  );
}
