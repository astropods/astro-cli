import type { Meta, StoryObj } from "@storybook/react-vite";

import { SuccessPanel } from "@/components/deploy/ErrorPanel";

const meta = {
  title: "Design System/Composites/SuccessPanel",
  component: SuccessPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof SuccessPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Deployment complete",
    children: "Your latest build is live and healthy across all services.",
  },
};

export const Inline: Story = {
  args: {
    title: "Deployment complete",
    children: "Your latest build is live and healthy across all services.",
    variant: "inline",
  },
};
