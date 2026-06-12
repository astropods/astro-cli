import type { MetaFunction } from "react-router";
import { Navigate } from "react-router";
import { hasExperiments, useExperiments } from "@/lib/experiments";
import { Switch } from "@/components/ui/switch";

export const meta: MetaFunction = () => [{ title: "Experiments - Settings | Astro" }];

export default function ExperimentsSettings() {
  const { experiments, setExperiment } = useExperiments();

  if (!hasExperiments) return <Navigate to="/settings/account" replace />;
  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-heading-4 text-foreground">Experiments</h2>
        <p className="mt-1 text-body-sm text-muted-foreground">Early-access features that may change or be removed.</p>
      </div>
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4 rounded-md border border-border bg-card px-4 py-3">
          <div>
            <p className="text-body-sm font-medium text-foreground">Eval tab</p>
            <p className="text-body-sm text-muted-foreground">Show the Eval tab on agent detail pages.</p>
          </div>
          <Switch
            checked={experiments.evals}
            onCheckedChange={(checked) => setExperiment("evals", checked)}
          />
        </div>
      </div>
    </div>
  );
}
