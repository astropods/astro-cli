import type { Meta, StoryObj } from "@storybook/react-vite";
import { PermissionsPreview } from "@/components/agent-detail/PermissionsPreview";

const meta = {
  title: "AgentDetail/PermissionsPreview",
  component: PermissionsPreview,
  decorators: [
    (Story) => (
      <div className="max-w-xs bg-stone-100 p-5 rounded-lg">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PermissionsPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const FewPermissions: Story = {
  args: {
    permissions: [
      "Read Slack messages",
      "Send Slack messages",
    ],
  },
};

export const ExactlyThree: Story = {
  args: {
    permissions: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
    ],
  },
};

export const WithCollapsible: Story = {
  args: {
    permissions: [
      "Read Slack messages",
      "Send Slack messages",
      "Access GitHub repositories",
      "Create Linear issues",
      "Read Google Drive files",
      "Send email notifications",
    ],
  },
};
