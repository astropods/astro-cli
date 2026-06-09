import { Link } from "react-router";
import { PlusIcon, RocketLaunchIcon } from "@heroicons/react/24/outline";
import { Telescope } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { blueprintsAccountPath, explorePath } from "@/lib/routes";

interface DashboardAgentsEmptyStateProps {
  /** Currently-selected account. Determines whether to show "Deploy a
   *  blueprint" (account has existing blueprints) or "Create blueprint"
   *  (no blueprints to deploy yet). */
  account: string;
}

export function DashboardAgentsEmptyState({ account }: DashboardAgentsEmptyStateProps) {
  // If the account already has blueprints, the primary action is to deploy
  // one of them — not to create a new one. This fetch is the only signal
  // for whether to show "Deploy" vs "Create".
  //
  // `useAccountBlueprints` is keyed on `blueprintKeys.byAccount(account)`
  // (= `['agents', 'account', <account>]`), so the cache entry is shared
  // with any other `useAccountBlueprints` caller for the same account
  // (e.g., the deployment history panel) — not with /blueprints, which
  // uses `blueprintKeys.list(account, params)` and lives under a longer
  // key. The two keys do share the `['agents', 'account', <account>]`
  // prefix, so mutations that invalidate by that prefix (create / delete
  // blueprint) refresh this query alongside the list view, even though
  // they don't share a cache row.
  const { data: blueprintsData } = useAccountBlueprints(account);
  const hasBlueprints = (blueprintsData?.agents?.length ?? 0) > 0;

  return (
    <div className="rounded-lg border border-dashed border-border-strong px-6 py-12 text-center">
      <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-md bg-border">
        <RocketLaunchIcon className="size-6 text-muted-foreground" />
      </div>
      <p className="text-heading-3 text-foreground mb-2">No agents deployed yet</p>
      <p className="text-body text-muted-foreground mb-6 max-w-sm mx-auto">
        {hasBlueprints
          ? "Pick one of your blueprints and deploy it to get an agent running."
          : "To deploy an agent you'll need a blueprint, a spec that defines what your agent does."}
      </p>
      <div className="flex flex-wrap items-center justify-center gap-3">
        {hasBlueprints ? (
          <Button asChild>
            <Link to={blueprintsAccountPath(account)}>
              <RocketLaunchIcon className="size-4" />
              Deploy a blueprint
            </Link>
          </Button>
        ) : (
          <Button asChild>
            <Link to="/getting-started">
              <PlusIcon className="size-4" />
              Create blueprint
            </Link>
          </Button>
        )}
        <Button variant="outline" asChild>
          <Link to={explorePath}>
            <Telescope className="size-4" strokeWidth={1.5} />
            Explore community
          </Link>
        </Button>
      </div>
    </div>
  );
}
