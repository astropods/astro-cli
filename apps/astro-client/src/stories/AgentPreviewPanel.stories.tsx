import type { Meta, StoryObj } from "@storybook/react-vite";

import { AgentPreviewPanel } from "@/components/AgentPreviewPanel";

const meta = {
  title: "Components/AgentPreviewPanel",
  component: AgentPreviewPanel,
  decorators: [
    (Story) => (
      <div className="h-[600px] flex">
        <Story />
      </div>
    ),
  ],
  // The component uses `hidden lg:flex` so override to always show
  args: {
    className: "!flex",
  },
} satisfies Meta<typeof AgentPreviewPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {
  args: {
    suggestedPrompts: [],
  },
};

export const WithPrompts: Story = {
  args: {
    suggestedPrompts: [
      "Review recent incidents",
      "Analyze the top 3 alerts",
      "Analyze churn feedback",
    ],
  },
};
