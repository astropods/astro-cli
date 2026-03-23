import type { Meta, StoryObj } from "@storybook/react-vite";

import { WarningPanel } from "@/components/ui/status-panel";

const meta = {
  title: "Design System/Composites/WarningPanel",
  component: WarningPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof WarningPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Deployment delayed",
    children: "We are retrying container startup after a temporary infrastructure issue.",
  },
};

export const Inline: Story = {
  args: {
    title: "Deployment delayed",
    children: "We are retrying container startup after a temporary infrastructure issue.",
    variant: "inline",
  },
};

export const Dismissible: Story = {
  args: {
    title: "Deployment delayed",
    children: "We are retrying container startup after a temporary infrastructure issue.",
    dismissible: true,
  },
};
