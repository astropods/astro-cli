import { useBlueprints } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { PageContainer, PageHeader } from "@/components/PageLayout";
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
    <PageContainer outerClassName="bg-stone-100">
      <PageHeader title="Explore community blueprints" />
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
    </PageContainer>
  );
}
