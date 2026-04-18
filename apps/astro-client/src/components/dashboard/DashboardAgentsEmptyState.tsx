import { Link } from "react-router";
import { PlusIcon, GlobeAltIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { explorePath } from "@/lib/routes";

export function DashboardAgentsEmptyState() {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <p className="text-base font-semibold text-foreground mb-2">No agents deployed yet</p>
      <p className="text-sm text-muted-foreground mb-6 max-w-sm mx-auto">
        To deploy an agent you'll need a blueprint, a spec that defines what your agent does. Create your own or explore ones built by the community.
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
