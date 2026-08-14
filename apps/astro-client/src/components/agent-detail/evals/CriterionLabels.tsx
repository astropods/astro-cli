import { Tag } from "@/components/Tag";
import { OverflowPopover } from "@/components/activity/OverflowPopover";
import type { JudgmentCriterion } from "@/lib/api";
import { criterionLabelFor } from "./judgment-criteria";

export interface CriterionLabelsProps {
  criteria: JudgmentCriterion[];
}

/** Renders the first criterion label plus a `+N` overflow chip that reveals the
 *  full set on hover. Shows a faint placeholder when there are no known
 *  criteria. */
export function CriterionLabels({ criteria }: CriterionLabelsProps) {
  const labels = criteria
    .filter((criterion) => criterion.value > 0)
    .map((c) => criterionLabelFor(c.dimension_key, c.value))
    .filter((label): label is string => label !== null);

  if (labels.length === 0) {
    return <span className="text-faint-foreground">—</span>;
  }

  const [first, ...rest] = labels;

  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <span className="truncate">
        <Tag color="default">{first}</Tag>
      </span>
      {rest.length > 0 && (
        <OverflowPopover
          overflow={rest.length}
          total={labels.length}
          itemNoun={{ singular: "criterion", plural: "criteria" }}
          trigger="hover"
        >
          <div className="flex min-h-0 flex-wrap gap-2 overflow-y-auto">
            {labels.map((label) => (
              <Tag key={label} color="default">
                {label}
              </Tag>
            ))}
          </div>
        </OverflowPopover>
      )}
    </div>
  );
}
