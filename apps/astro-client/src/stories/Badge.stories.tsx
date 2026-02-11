import type { Meta, StoryObj } from "@storybook/react-vite";

import { Badge } from "@/components/Badge";

const meta = {
  title: "Components/Badge",
  component: Badge,
} satisfies Meta<typeof Badge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { children: "Marketing" },
};

export const Active: Story = {
  args: { variant: "active", showDot: true, children: "active" },
};

export const Pending: Story = {
  args: { variant: "pending", showDot: true, children: "pending" },
};

export const Inactive: Story = {
  args: { variant: "inactive", showDot: true, children: "inactive" },
};

export const Warning: Story = {
  args: { variant: "warning", showDot: true, children: "warning" },
};

export const Error: Story = {
  args: { variant: "error", showDot: true, children: "error" },
};

export const Info: Story = {
  args: { variant: "info", showDot: true, children: "info" },
};

export const AllVariants: Story = {
  name: "All Variants",
  args: { children: "default" },
  render: () => (
    <div className="flex items-center gap-3">
      <Badge>default</Badge>
      <Badge variant="active" showDot>active</Badge>
      <Badge variant="pending" showDot>pending</Badge>
      <Badge variant="inactive" showDot>inactive</Badge>
      <Badge variant="warning" showDot>warning</Badge>
      <Badge variant="error" showDot>error</Badge>
      <Badge variant="info" showDot>info</Badge>
    </div>
  ),
};

export const CategoryBadges: Story = {
  name: "Category Badges",
  args: { children: "Marketing" },
  render: () => (
    <div className="flex items-center gap-1">
      <Badge>Marketing</Badge>
      <Badge>Sales</Badge>
    </div>
  ),
};
