import type { TraceDetail, TraceScore } from "@/lib/api";
import { ContentSection } from "./ContentSection";
import { MetadataList } from "./MetadataList";
import { TagsRow } from "./TagsRow";
import { ScoresList } from "./ScoresList";

export interface TraceOverviewTabProps {
  trace: TraceDetail;
  scores: TraceScore[];
}

export function TraceOverviewTab({ trace, scores }: TraceOverviewTabProps) {
  type MetaEntry = { label: string; value: React.ReactNode };
  const meta = (
    [
      trace.name ? { label: "Name", value: trace.name } : null,
      trace.session_id ? { label: "Session", value: trace.session_id } : null,
      trace.user_id ? { label: "User", value: trace.user_id } : null,
      trace.environment ? { label: "Environment", value: trace.environment } : null,
      trace.release ? { label: "Release", value: trace.release } : null,
      trace.version ? { label: "Version", value: trace.version } : null,
    ] as Array<MetaEntry | null>
  ).filter((x): x is MetaEntry => x !== null);

  return (
    <div className="flex flex-col gap-3">
      <ContentSection label="Input" content={trace.input} />
      <ContentSection label="Output" content={trace.output} />

      <TagsRow tags={trace.tags} />

      <MetadataList entries={meta} />

      {trace.metadata && Object.keys(trace.metadata).length > 0 && (
        <ContentSection
          label="Metadata"
          content={trace.metadata}
          defaultOpen={false}
        />
      )}

      <ScoresList scores={scores} />
    </div>
  );
}
