import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarStats } from "@/components/agent-detail/SidebarStats";

const meta = {
  title: "AgentDetail/SidebarStats",
  component: SidebarStats,
  decorators: [
    (Story) => (
      <div className="max-w-xs bg-stone-100 p-5 rounded-lg">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SidebarStats>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SemverAndDate: Story = {
  args: {
    version: "1.2.3",
    isSemver: true,
    updatedAt: "Jan 15, 2026",
  },
};

export const BuildIdAndDate: Story = {
  args: {
    version: "a1b2c3d4",
    isSemver: false,
    updatedAt: "Feb 20, 2026",
  },
};

export const VersionOnly: Story = {
  args: {
    version: "2.0.0",
    isSemver: true,
  },
};

export const DateOnly: Story = {
  args: {
    updatedAt: "Dec 5, 2025",
  },
};
