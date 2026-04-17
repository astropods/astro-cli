import { useBlueprints } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { useAuth } from "@/lib/auth";

export async function loader({ request }: { request: Request }) {
  const api = createServerApi(request);
  const blueprintsData = await api.listBlueprints().catch(() => ({ agents: [], count: 0 }));
  return { blueprintsData };
}

export function meta() {
  return [
    { title: "Explore | Astro" },
    { name: "description", content: "Explore public AI agent blueprints on Astro." },
    { property: "og:title", content: "Explore | Astro" },
    { property: "og:description", content: "Explore public AI agent blueprints on Astro." },
  ];
}

export default function Explore({ loaderData }: { loaderData: Awaited<ReturnType<typeof loader>> }) {
  const { data, isLoading, isError, error, refetch } = useBlueprints({
    initialData: loaderData?.blueprintsData,
  });
  const { accounts } = useAuth();
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-6 pb-6 pt-8 md:px-8 md:pb-8 md:pt-10 max-w-[1500px] mx-auto">
      <h1 className="mb-6 text-heading-1 text-foreground">Explore community blueprints</h1>
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
    </div>
  );
}
