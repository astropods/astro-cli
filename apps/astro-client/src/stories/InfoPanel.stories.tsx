import type { Meta, StoryObj } from "@storybook/react-vite";

import { InfoPanel } from "@/components/deploy/ErrorPanel";

const meta = {
  title: "Design System/Composites/InfoPanel",
  component: InfoPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof InfoPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    title: "Heads up",
    children: "Your deployment is still warming up. Some metrics may take a minute to appear.",
  },
};

export const Inline: Story = {
  args: {
    title: "Heads up",
    children: "Your deployment is still warming up. Some metrics may take a minute to appear.",
    variant: "inline",
  },
};
