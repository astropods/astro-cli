import type { Meta, StoryObj } from "@storybook/react-vite";

import { StatusBadge } from "@/components/StatusBadge";

const meta = {
  title: "Components/StatusBadge",
  component: StatusBadge,
} satisfies Meta<typeof StatusBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Active: Story = {
  args: { variant: "active" },
};

export const Pending: Story = {
  args: { variant: "pending" },
};

export const Inactive: Story = {
  args: { variant: "inactive" },
};

export const Warning: Story = {
  args: { variant: "warning" },
};

export const Error: Story = {
  args: { variant: "error" },
};

export const Info: Story = {
  args: { variant: "info" },
};

export const NoDot: Story = {
  name: "No Dot",
  args: { variant: "active", showDot: false },
};

export const AllVariants: Story = {
  name: "All Variants",
  render: () => (
    <div className="flex items-center gap-3">
      <StatusBadge variant="active" />
      <StatusBadge variant="pending" />
      <StatusBadge variant="inactive" />
      <StatusBadge variant="warning" />
      <StatusBadge variant="error" />
      <StatusBadge variant="info" />
    </div>
  ),
};

export const AllVariantsNoDot: Story = {
  name: "All Variants (No Dot)",
  render: () => (
    <div className="flex items-center gap-3">
      <StatusBadge variant="active" showDot={false} />
      <StatusBadge variant="pending" showDot={false} />
      <StatusBadge variant="inactive" showDot={false} />
      <StatusBadge variant="warning" showDot={false} />
      <StatusBadge variant="error" showDot={false} />
      <StatusBadge variant="info" showDot={false} />
    </div>
  ),
};
