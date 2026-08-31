import { Tag } from "@/components/Tag";
import { OverflowPopover } from "@/components/activity/OverflowPopover";

interface ValueLabelsProps {
  labels: string[];
  itemNoun: { singular: string; plural: string };
}

export function ValueLabels({ labels, itemNoun }: ValueLabelsProps) {
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
          itemNoun={itemNoun}
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
