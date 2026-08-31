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
import type { EvalDatasetItem, EvaluationSetEvaluator } from "@/lib/api";

const reviewer: ResolvedReviewer = {
  handle: "reviewer",
  name: "Riley Chen",
};

const evaluators: EvaluationSetEvaluator[] = [
  {
    key: "exposed_pii",
    label: "Exposed PII",
    description: "Flags personal data in the output.",
    type: "llm",
    output: { type: "boolean" },
  },
  {
    key: "user_sentiment",
    label: "User sentiment",
    type: "llm",
    output: { type: "enum", options: ["positive", "neutral", "negative"] },
  },
];

const item = (overrides: Partial<EvalDatasetItem> = {}): EvalDatasetItem => ({
  id: "item-1",
  input: {
    message: "Can you help me deploy this agent to production?",
    channel: "chat",
  },
  expected_output:
    "Yes. Run `ast deploy` from the project root, then watch the deployment status in Astro.",
  source_trace_id: "trace-123456",
  created_at: "2026-06-23T14:29:00Z",
  evaluation_ref: "preset/default-set",
  verified_by_user_id: "user-1",
  evaluator_outputs: [
    { key: "exposed_pii", label: "Exposed PII", value: false },
    { key: "user_sentiment", label: "User sentiment", value: "positive" },
  ],
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
              Verified by
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <DatasetItemRow
            item={row}
            evaluators={evaluators}
            evaluationRef="preset/default-set"
            evaluatorsUnavailable={false}
            isOpen={open}
            onToggle={() => setOpen((current) => !current)}
            onRemove={() => undefined}
            onSaveOutputs={(_traceId, _outputs, onSaved) => onSaved()}
            isRemoving={false}
            isSavingOutputs={false}
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

export const OlderEvaluationSet: Story = {
  render: () => (
    <DatasetRowStory
      defaultOpen
      row={item({
        id: "item-2",
        evaluation_ref: "preset/retired-set",
      })}
    />
  ),
};
