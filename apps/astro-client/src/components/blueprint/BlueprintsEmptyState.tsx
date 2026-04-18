import { Link } from "react-router";
import { ArrowRightIcon, CommandLineIcon, PlusIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { BlueprintCard } from "@/components/BlueprintCard";
import { getBlueprintDescription } from "@/lib/blueprint-utils";
import { explorePath } from "@/lib/routes";
import { useBlueprints } from "@/api/queries";
import { AgentMascots } from "@/components/AgentMascots";

export function BlueprintsEmptyState() {
  const { data: blueprintsData } = useBlueprints();
  const communityBlueprints = (blueprintsData?.agents ?? [])
    .slice()
    .sort((a, b) => (b.heart_count ?? 0) - (a.heart_count ?? 0))
    .slice(0, 4);

  return (
    <div className="flex flex-col gap-6">
      <div
        className="flex flex-col items-center rounded-xl border border-border bg-background px-6 py-14 text-center"
        style={{
          backgroundImage:
            "radial-gradient(ellipse 120% 80% at 50% 0%, color-mix(in oklch, var(--muted) 55%, transparent) 0%, transparent 55%), radial-gradient(ellipse 90% 70% at 80% 100%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 50%)",
        }}
      >
        <div className="mb-4">
          <AgentMascots size={36} />
        </div>
        <h2 className="mb-2 text-heading-3 text-foreground">No blueprints yet</h2>
        <p className="mb-6 max-w-sm text-body text-muted-foreground">
          Blueprints define what your agent does. Create your own or start from a community blueprint.
        </p>
        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button asChild>
            <Link to="/new/custom">
              <PlusIcon className="size-4 text-current" />
              Create blueprint
            </Link>
          </Button>
          <Button variant="outline" asChild>
            <Link to="https://docs.astropods.com/install-cli" target="_blank" rel="noopener noreferrer">
              <CommandLineIcon className="size-4" />
              Start from CLI
            </Link>
          </Button>
        </div>
      </div>

      <div>
        <div className="mb-3 flex items-center justify-between gap-4">
          <h2 className="text-heading-3 text-foreground">Explore community blueprints</h2>
          <Button variant="ghost" size="sm" asChild>
            <Link to={explorePath} className="gap-1">
              View all
              <ArrowRightIcon className="size-3.5" />
            </Link>
          </Button>
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {communityBlueprints.map((bp) => (
            <BlueprintCard
              key={`${bp.account}/${bp.name}`}
              slug={`${bp.account}/${bp.name}`}
              account={bp.account}
              name={bp.name}
              description={getBlueprintDescription(bp)}
              deployCount={bp.heart_count ?? 0}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
