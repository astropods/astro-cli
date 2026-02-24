import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarAbout } from "@/components/agent-detail/SidebarAbout";

const meta = {
  title: "AgentDetail/SidebarAbout",
  component: SidebarAbout,
  decorators: [
    (Story) => (
      <div className="max-w-xs bg-stone-100 p-5 rounded-lg">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SidebarAbout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    description:
      "Analyzes customer feedback to surface actionable insights and trends across all channels.",
  },
};

export const Long: Story = {
  args: {
    description:
      "This agent monitors your production environment 24/7, detects anomalies in real-time, and automatically triages incidents based on severity. It integrates with your existing alerting stack and can escalate critical issues to on-call engineers via Slack or PagerDuty.",
  },
};
