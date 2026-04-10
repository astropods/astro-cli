import type { ReactNode } from "react";
import { useAccountBlueprints } from "@/api/queries";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import type { BlueprintsListResponse } from "@/lib/api";

export function AccountBlueprintsList({ account, initialData, emptyContent, variant }: { account: string; initialData?: BlueprintsListResponse; emptyContent?: ReactNode; variant?: "grid" | "list" }) {
  const { data, isLoading, isError, error, refetch } = useAccountBlueprints(account, {
    initialData,
  });

  return (
    <BlueprintListView
      blueprints={data?.agents ?? []}
      isLoading={isLoading}
      isError={isError}
      error={error}
      refetch={refetch}
      emptyTitle="No blueprints yet"
      emptyDescription="This account has no blueprints in the registry."
      emptyContent={emptyContent}
      ownerAccounts={new Set([account])}
      variant={variant}
    />
  );
}
