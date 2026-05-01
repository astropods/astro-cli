import { useEffect } from "react";
import { Link, useSearchParams } from "react-router";
import { PlusIcon } from "@heroicons/react/24/outline";
import { useAccountBlueprints } from "@/api/queries";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { BlueprintsEmptyState } from "@/components/blueprint/BlueprintsEmptyState";
import { PageScopeSwitcher } from "@/components/PageScopeSwitcher";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { getPersonalAccount } from "@/lib/api.server";
import type { Route } from "./+types/Blueprints";

export const meta: Route.MetaFunction = () => [{ title: "Blueprints | Astro" }];

export async function loader({ request }: Route.LoaderArgs) {
  const ctx = await getPersonalAccount(request);
  if (!ctx) return { count: 0 };
  return ctx.api.listAccountBlueprints(ctx.accountName).catch(() => ({ count: 0 }));
}

export default function Blueprints({ loaderData }: Route.ComponentProps) {
  const { activeAccount, setActiveAccount } = useActiveAccount();
  const { accounts, isAuthenticated } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();

  // Consume ?account= param once accounts have loaded so deep-links and
  // org-switcher redirects land on the right scope without a flash.
  useEffect(() => {
    const accountParam = searchParams.get("account");
    if (!accountParam || accounts.length === 0) return;
    if (accounts.some((a) => a.name === accountParam)) {
      setActiveAccount(accountParam);
    }
    setSearchParams({}, { replace: true });
  }, [accounts, searchParams, setActiveAccount, setSearchParams]);
  const isReady = isAuthenticated && !!activeAccount;
  const { data, isLoading, isError, error, refetch } = useAccountBlueprints(activeAccount, {
    enabled: isReady,
  });
  const ownerAccounts = new Set(accounts.map((a) => a.name));

  return (
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Blueprints"
        description="Agent configurations available to deploy in your account."
        action={
          isAuthenticated && (
            <div className="flex w-full flex-wrap items-center gap-3 sm:w-auto">
              <PageScopeSwitcher />
              <Button asChild size="sm">
                <Link to="/new/custom">
                  <PlusIcon className="size-4" />
                  Create blueprint
                </Link>
              </Button>
            </div>
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
        skeletonCount={loaderData?.count}
        showAuthor
      />
    </PageContainer>
  );
}
