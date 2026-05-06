import type { MetaFunction } from "react-router";
import { FlaskConical } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { useExperiments } from "@/lib/experiments";
import { SectionHeader } from "@/components/settings/SettingsShared";

export const meta: MetaFunction = () => [{ title: "Experiments - Settings | Astro" }];

interface ExperimentRowProps {
  title: string;
  description: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}

function ExperimentRow({ title, description, checked, onCheckedChange }: ExperimentRowProps) {
  return (
    <div className="flex items-start justify-between gap-4 py-4 border-b border-border last:border-0">
      <div className="space-y-0.5">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-[13px] text-muted-foreground">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} className="mt-0.5 shrink-0" />
    </div>
  );
}

export default function ExperimentsSettings() {
  const { experiments, setExperiment } = useExperiments();

  return (
    <>
      <SectionHeader
        title={<span className="flex items-center gap-2"><FlaskConical className="size-4" />Experimental features</span>}
        subtitle="These features are in development and may change or be removed. Preferences are stored locally in your browser."
      />

      <div className="rounded-md border border-border bg-surface px-4">
        <ExperimentRow
          title="Theming"
          description="Enable light, dark, and auto theme switching from the user menu."
          checked={experiments.theming}
          onCheckedChange={(v) => setExperiment("theming", v)}
        />
      </div>
    </>
  );
}
