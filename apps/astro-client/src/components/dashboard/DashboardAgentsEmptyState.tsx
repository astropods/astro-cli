import { Link } from "react-router";
import { PlusIcon, MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { explorePath } from "@/lib/routes";

export function DashboardAgentsEmptyState() {
  return (
    <div className="rounded-lg border border-dashed border-border px-6 py-12 text-center">
      <p className="text-sm font-medium text-foreground mb-1">No agents deployed yet</p>
      <p className="text-xs text-muted-foreground mb-4">
        Blueprints define what your agent does. Choose one from the community to deploy instantly, or build your own.
      </p>
      <div className="flex items-center justify-center gap-2">
        <Button size="sm" asChild>
          <Link to="/getting-started">
            <PlusIcon className="size-3.5" />
            Create blueprint
          </Link>
        </Button>
        <Button size="sm" variant="outline" asChild>
          <Link to={explorePath}>
            <MagnifyingGlassIcon className="size-3.5" />
            Explore community blueprints
          </Link>
        </Button>
      </div>
    </div>
  );
}
