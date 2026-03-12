import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { AgentDetailBreadcrumb } from "@/components/agent-detail/AgentDetailBreadcrumb";

const meta = {
  title: "Features/Agents/AgentDetailBreadcrumb",
  component: AgentDetailBreadcrumb,
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof AgentDetailBreadcrumb>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    account: "sohumonlocal",
    agentName: "api-changelog-writer",
  },
};
