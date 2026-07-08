import type { Meta, StoryObj } from "@storybook/react-vite";
import { DatasetGrade } from "@/components/agent-detail/evals/dataset/DatasetGrade";

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

export const Ring: Story = {
  args: {
    grade: "C",
    variant: "ring",
    nextGrade: "B",
    progress: 0.67,
  },
};

export const RingTopGrade: Story = {
  args: {
    grade: "A",
    variant: "ring",
    nextGrade: "",
    progress: 1,
  },
};
