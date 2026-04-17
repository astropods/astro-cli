import type { MetaFunction } from "react-router";
import { FlaskConical } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { useExperiments } from "@/lib/experiments";

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
    <div className="space-y-8">
      <div className="space-y-1">
        <h1 className="text-heading-1 text-foreground flex items-center gap-2">
          <FlaskConical className="size-5" />
          Experimental features
        </h1>
        <p className="text-[13px] text-muted-foreground">
          These features are in development and may change or be removed. Preferences are stored locally in your browser.
        </p>
      </div>

      <div className="rounded-md border border-border-strong bg-surface px-4">
        <ExperimentRow
          title="Knowledge Stores"
          description="Provision and connect databases (Postgres, Redis, Qdrant, Neo4j, Pinecone) for agent memory, vector search, and caching."
          checked={experiments.knowledgeStore}
          onCheckedChange={(v) => setExperiment("knowledgeStore", v)}
        />
        <ExperimentRow
          title="Theming"
          description="Enable light, dark, and auto theme switching from the user menu."
          checked={experiments.theming}
          onCheckedChange={(v) => setExperiment("theming", v)}
        />
      </div>
    </div>
  );
}
