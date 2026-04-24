import { Link } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { useAccountBlueprints } from "@/api/queries";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { BlueprintsEmptyState } from "@/components/blueprint/BlueprintsEmptyState";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";

export function meta() {
  return [{ title: "Blueprints | Astro" }];
}

export default function Blueprints() {
  const { activeAccount } = useActiveAccount();
  const { accounts, isAuthenticated } = useAuth();
  const isReady = isAuthenticated && !!activeAccount;
  const { data, isLoading, isError, error, refetch } = useAccountBlueprints(activeAccount, {
    enabled: isReady,
  });
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  return (
    <PageContainer outerClassName="bg-stone-100 dark:bg-muted">
      <PageHeader
        title="Blueprints"
        description="Agent configurations available to deploy in your account."
        adornment={<PageScopeSwitcher />}
        action={
          isAuthenticated && (
            <Button asChild size="sm">
              <Link to="/new/custom">
                <PlusIcon className="size-4" />
                Create blueprint
              </Link>
            </Button>
          )
        }
      />
      <BlueprintListView
        blueprints={data?.agents ?? []}
        isLoading={!isReady || isLoading}
        isError={isError}
        error={error}
        refetch={refetch}
        emptyContent={<BlueprintsEmptyState />}
        ownerAccounts={ownerAccounts}
      />
    </PageContainer>
  );
}
