import type { Meta, StoryObj } from "@storybook/react-vite";
import { DatasetGrade } from "@/components/agent-detail/evals/DatasetGrade";

const meta = {
  title: "Features/Agents/Evals/DatasetGrade",
  component: DatasetGrade,
  args: {
    grade: "B",
  },
} satisfies Meta<typeof DatasetGrade>;

export default meta;
type Story = StoryObj<typeof meta>;

export const BadgeStates: Story = {
  render: () => (
    <div className="flex items-center gap-3">
      {["A", "B", "C", "D", "F", "—"].map((grade) => (
        <DatasetGrade key={grade} grade={grade} />
      ))}
    </div>
  ),
};

export const LabelWithProgress: Story = {
  args: {
    grade: "C",
    variant: "label",
    itemCount: 24,
    nextGrade: "B",
    nextGradeProgress: 0.68,
  },
};

export const EmptyLabel: Story = {
  args: {
    grade: "—",
    variant: "label",
    itemCount: 0,
  },
};
