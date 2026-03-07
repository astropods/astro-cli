import type { Meta, StoryObj } from "@storybook/react-vite";

import { ErrorPanel } from "@/components/deploy/ErrorPanel";

const meta = {
  title: "Design System/Composites/ErrorPanel",
  component: ErrorPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ErrorPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Deployment failed",
    children: "Connection to the deployment service timed out. Please try again.",
  },
};

export const MultilineError: Story = {
  name: "Multiline Error",
  args: {
    title: "Deployment failed",
    children:
      "SLACK_BOT_TOKEN: invalid token format\nSLACK_APP_TOKEN: token expired\nMissing credentials: DATABASE_URL",
  },
};

export const WithoutTitle: Story = {
  name: "Without Title",
  args: {
    children: "Something went wrong during deployment.",
  },
};

