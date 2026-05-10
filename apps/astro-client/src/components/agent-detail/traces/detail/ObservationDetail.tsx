import { ArrowLeft } from "lucide-react";
import type { TraceObservation } from "@/lib/api";
import { formatCost, formatLatency } from "../trace-utils";
import { observationTypeLabel } from "./observation-utils";
import { ContentSection } from "./ContentSection";
import { MetadataList } from "./MetadataList";

export interface ObservationDetailProps {
  observation: TraceObservation;
  onBack: () => void;
}

export function ObservationDetail({ observation, onBack }: ObservationDetailProps) {
  type MetaEntry = { label: string; value: React.ReactNode };
  const meta = (
    [
      { label: "Type", value: observationTypeLabel(observation.type) },
      { label: "Latency", value: formatLatency(observation.latency_ms, true) },
      observation.model ? { label: "Model", value: observation.model } : null,
      observation.cost && observation.cost > 0
        ? { label: "Cost", value: formatCost(observation.cost) }
        : null,
      observation.usage
        ? {
            label: "Tokens",
            value: `${observation.usage.input.toLocaleString()} in · ${observation.usage.output.toLocaleString()} out`,
          }
        : null,
      observation.level && observation.level !== "default"
        ? { label: "Level", value: observation.level }
        : null,
      observation.status_message
        ? { label: "Status", value: observation.status_message }
        : null,
    ] as Array<MetaEntry | null>
  ).filter((x): x is MetaEntry => x !== null);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-mono-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-3" />
          Back to tree
        </button>
      </div>

      <div>
        <h4 className="text-heading-4 text-foreground">
          {observation.name || <em className="text-muted-foreground">unnamed</em>}
        </h4>
        <p className="mt-0.5 font-mono text-mono-sm text-muted-foreground/60">
          {observation.id}
        </p>
      </div>

      <MetadataList entries={meta} />

      {observation.model_parameters && (
        <ContentSection
          label="Model parameters"
          content={observation.model_parameters}
          defaultOpen={false}
        />
      )}

      <ContentSection label="Input" content={observation.input} />
      <ContentSection label="Output" content={observation.output} />

      {observation.metadata && Object.keys(observation.metadata).length > 0 && (
        <ContentSection
          label="Metadata"
          content={observation.metadata}
          defaultOpen={false}
        />
      )}
    </div>
  );
}
