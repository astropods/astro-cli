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
  args: { children: "success" },
  render: () => (
    <div className="flex items-center gap-2">
      <InlineBadge
        variant="soft"
        style={{
          color: "var(--color-teal-600)",
          background: "color-mix(in oklch, var(--color-teal-600) 12%, transparent)",
        }}
      >
        success
      </InlineBadge>
      <InlineBadge
        variant="soft"
        style={{
          color: "var(--color-yellow-700)",
          background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)",
        }}
      >
        timeout
      </InlineBadge>
      <InlineBadge
        variant="soft"
        style={{
          color: "var(--color-red-700)",
          background: "color-mix(in oklch, var(--color-red-700) 12%, transparent)",
        }}
      >
        error
      </InlineBadge>
    </div>
  ),
};
