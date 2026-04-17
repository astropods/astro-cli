import { Link } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { useAccountBlueprints } from "@/api/queries";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { DashboardAgentsEmptyState } from "@/components/dashboard/DashboardAgentsEmptyState";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";

export function meta() {
  return [{ title: "Blueprints | Astro" }];
}

export default function Blueprints() {
  const { activeAccount } = useActiveAccount();
  const { accounts, isAuthenticated } = useAuth();
  const { data, isLoading, isError, error, refetch } = useAccountBlueprints(activeAccount, {
    enabled: isAuthenticated && !!activeAccount,
  });
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-6 pb-6 pt-8 md:px-8 md:pb-8 md:pt-10 max-w-[1500px] mx-auto">
      <div className="mb-6 flex items-start justify-between">
        <div>
          <h1 className="text-heading-1 text-foreground">Blueprints</h1>
          <p className="mt-1 text-[13px] text-muted-foreground">Agent configurations available to deploy in your account.</p>
        </div>
        {isAuthenticated && (
          <Button asChild size="sm">
            <Link to="/new/custom">
              <PlusIcon className="size-4" />
              Create blueprint
            </Link>
          </Button>
        )}
      </div>
      <BlueprintListView
        blueprints={data?.agents ?? []}
        isLoading={isLoading}
        isError={isError}
        error={error}
        refetch={refetch}
        emptyContent={<DashboardAgentsEmptyState />}
        ownerAccounts={ownerAccounts}
      />
    </div>
  );
}
