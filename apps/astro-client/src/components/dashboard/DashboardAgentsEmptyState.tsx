import { Link } from "react-router";
import { PlusIcon, GlobeAltIcon, RocketLaunchIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { explorePath } from "@/lib/routes";

export function DashboardAgentsEmptyState() {
  return (
    <div className="rounded-lg border border-dashed border-border-strong px-6 py-12 text-center">
      <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-xl bg-muted">
        <RocketLaunchIcon className="size-6 text-muted-foreground" />
      </div>
      <p className="text-heading-3 text-foreground mb-2">No agents deployed yet</p>
      <p className="text-body text-muted-foreground mb-6 max-w-sm mx-auto">
        To deploy an agent you'll need a blueprint, a spec that defines what your agent does.
      </p>
      <div className="flex items-center justify-center gap-3">
        <Button asChild>
          <Link to="/getting-started">
            <PlusIcon className="size-4" />
            Create blueprint
          </Link>
        </Button>
        <Button variant="outline" asChild>
          <Link to={explorePath}>
            <GlobeAltIcon className="size-4" />
            Explore community blueprints
          </Link>
        </Button>
      </div>
    </div>
  );
}
