import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DatasetItemRow,
  type ResolvedReviewer,
} from "@/components/agent-detail/evals/dataset/DatasetItemRow";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { EvalDatasetItem } from "@/lib/api";

const reviewer: ResolvedReviewer = {
  handle: "reviewer",
  name: "Riley Chen",
};

const item = (overrides: Partial<EvalDatasetItem> = {}): EvalDatasetItem => ({
  id: "item-1",
  input: {
    message: "Can you help me deploy this agent to production?",
    channel: "chat",
  },
  expected_output:
    "Yes. Run `ast deploy` from the project root, then watch the deployment status in Astro.",
  metadata: {
    verdict: 1,
    judged_by_user_id: "user-1",
    judged_at: "2026-06-23T14:30:00Z",
  },
  source_trace_id: "trace-123456",
  created_at: "2026-06-23T14:29:00Z",
  ...overrides,
});

function DatasetRowStory({
  defaultOpen = false,
  row = item(),
}: {
  defaultOpen?: boolean;
  row?: EvalDatasetItem;
}) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="max-w-5xl overflow-hidden rounded-lg border border-border">
      <Table bare>
        <TableHeader className="bg-black/2 dark:bg-white/3">
          <TableRow>
            <TableHead className="w-4 pl-5 pr-0 text-faint-foreground" />
            <TableHead className="text-faint-foreground">Input</TableHead>
            <TableHead className="text-faint-foreground">
              Expected output
            </TableHead>
            <TableHead className="w-[220px] pr-5 text-faint-foreground">
              Judged by
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <DatasetItemRow
            item={row}
            isOpen={open}
            onToggle={() => setOpen((current) => !current)}
            onRemove={() => undefined}
            onSaveCriteria={(_traceId, _criteria, onSaved) => onSaved()}
            isRemoving={false}
            isSavingCriteria={false}
            reviewer={reviewer}
          />
        </TableBody>
      </Table>
    </div>
  );
}

const meta = {
  title: "Features/Agents/Evals/DatasetItemRow",
  component: DatasetRowStory,
} satisfies Meta<typeof DatasetRowStory>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Collapsed: Story = {
  render: () => <DatasetRowStory />,
};

export const Expanded: Story = {
  render: () => <DatasetRowStory defaultOpen />,
};

export const BadExample: Story = {
  render: () => (
    <DatasetRowStory
      defaultOpen
      row={item({
        id: "item-2",
        metadata: {
          verdict: -1,
          judged_by_user_id: "user-1",
          judged_at: "2026-06-23T14:35:00Z",
        },
      })}
    />
  ),
};
