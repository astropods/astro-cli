import type { Meta, StoryObj } from "@storybook/react-vite";
import { SidebarDeployedAgents } from "@/components/blueprint-detail/SidebarDeployedAgents";

const meta = {
  title: "Features/Agents/SidebarDeployedAgents",
  component: SidebarDeployedAgents,
  decorators: [
    (Story) => (
      <div className="flex min-h-[400px] justify-end bg-surface p-4">
        <div className="w-[320px]">
          <Story />
        </div>
      </div>
    ),
  ],
} satisfies Meta<typeof SidebarDeployedAgents>;

export default meta;
type Story = StoryObj<typeof meta>;

// account="testuser" matches the mock auth context's membership, and the
// buildIds line up with the MSW deployments handler so rows render with their
// build IDs.
export const Default: Story = {
  args: {
    account: "testuser",
    blueprintName: "code-reviewer",
    buildIds: ["b2c3d4e5f6a7", "c3d4e5f6a7b8", "a1b2c3d4e5f6"],
  },
};

export const SingleDeployment: Story = {
  args: {
    account: "testuser",
    blueprintName: "code-reviewer",
    buildIds: ["b2c3d4e5f6a7"],
  },
};
