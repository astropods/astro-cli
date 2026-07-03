import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmptyState } from "@/components/EmptyState";

const meta = {
  title: "Design System/Composites/EmptyState",
  component: EmptyState,
  decorators: [
    (Story) => (
      <div className="flex h-[400px] w-[600px]">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof EmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "No agents yet",
    description: "Browse available agents and add one to get started.",
    actionLabel: "Browse blueprints",
    actionTo: "/blueprints",
  },
};

export const CardVariant: Story = {
  args: {
    title: "No results",
    description: "No agents match the current filters.",
    variant: "card",
    actions: [{ label: "Browse blueprints", to: "/blueprints" }],
  },
};
