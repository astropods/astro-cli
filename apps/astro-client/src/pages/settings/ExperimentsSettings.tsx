import type { MetaFunction } from "react-router";
import { Navigate } from "react-router";
import { hasExperiments, useExperiments } from "@/lib/experiments";
import { Switch } from "@/components/ui/switch";
import { SectionHeader } from "@/components/settings/SettingsShared";

export const meta: MetaFunction = () => [{ title: "Experiments - Settings | Astro" }];

export default function ExperimentsSettings() {
  const { experiments, setExperiment } = useExperiments();

  if (!hasExperiments) return <Navigate to="/settings/account" replace />;
  return (
    <>
      <SectionHeader title="Experiments" subtitle="Early-access features that may change or be removed" />
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4 rounded-md border border-border px-4 py-3">
          <div>
            <p className="text-body-sm font-medium text-foreground">Eval tab</p>
            <p className="text-body-sm text-muted-foreground">Show the Eval tab on agent detail pages.</p>
          </div>
          <Switch
            checked={experiments.evals}
            onCheckedChange={(checked) => setExperiment("evals", checked)}
          />
        </div>
        <div className="flex items-center justify-between gap-4 rounded-md border border-border px-4 py-3">
          <div>
            <p className="text-body-sm font-medium text-foreground">Faster Insights</p>
            <p className="text-body-sm text-muted-foreground">
              Load the Insights page from stored daily usage instead of recalculating it each
              time. Numbers should match; today&apos;s usage may still be catching up.
            </p>
          </div>
          <Switch
            checked={experiments.insightsRollups}
            onCheckedChange={(checked) => setExperiment("insightsRollups", checked)}
          />
        </div>
      </div>
    </>
  );
}
