import type { Meta, StoryObj } from "@storybook/react-vite";
import { InlineBadge } from "@/components/InlineBadge";

const meta = {
  title: "Design System/Primitives/InlineBadge",
  component: InlineBadge,
} satisfies Meta<typeof InlineBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { children: "API Agent" },
};

export const Multiple: Story = {
  args: { children: "API Agent" },
  render: () => (
    <div className="flex items-center gap-2">
      <InlineBadge>API Agent</InlineBadge>
      <InlineBadge>Proven</InlineBadge>
      <InlineBadge>Elite</InlineBadge>
    </div>
  ),
};

export const Categories: Story = {
  args: { children: "Marketing" },
  render: () => (
    <div className="flex flex-wrap gap-1">
      <InlineBadge>Marketing</InlineBadge>
      <InlineBadge>Sales</InlineBadge>
      <InlineBadge>Data Analysis</InlineBadge>
    </div>
  ),
};

export const StatusVariants: Story = {
  args: { children: "SUCCESS" },
  render: () => (
    <div className="flex items-center gap-2">
      <InlineBadge className="text-green-700 bg-green-100 border-green-300 dark:text-green-300 dark:bg-green-900/35 dark:border-green-600/35">
        SUCCESS
      </InlineBadge>
      <InlineBadge className="text-yellow-700 bg-yellow-100 border-yellow-300 dark:text-yellow-300 dark:bg-yellow-900/35 dark:border-yellow-600/35">
        TIMEOUT
      </InlineBadge>
      <InlineBadge className="text-red-700 bg-red-100 border-red-300 dark:text-red-300 dark:bg-red-900/35 dark:border-red-600/35">
        ERROR
      </InlineBadge>
    </div>
  ),
};
