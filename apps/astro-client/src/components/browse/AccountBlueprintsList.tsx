import { useAccountBlueprints } from "@/api/queries";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import type { BlueprintsListResponse } from "@/lib/api";

export function AccountBlueprintsList({ account, initialData }: { account: string; initialData?: BlueprintsListResponse }) {
  const { data, isLoading, isError, error, refetch } = useAccountBlueprints(account, { initialData });

  return (
    <BlueprintListView
      blueprints={data?.blueprints ?? []}
      isLoading={isLoading}
      isError={isError}
      error={error}
      refetch={refetch}
      emptyTitle="No blueprints yet"
      emptyDescription="This account has no blueprints in the registry."
    />
  );
}
