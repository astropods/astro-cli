import type { Meta, StoryObj } from "@storybook/react-vite";
import { StatusBadge } from "@/components/StatusBadge";

const meta = {
  title: "Design System/Primitives/StatusBadge",
  component: StatusBadge,
  args: { color: "success", children: "Active" },
  argTypes: {
    size: {
      control: "select",
      options: ["sm", "md"],
    },
  },
} satisfies Meta<typeof StatusBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Colors: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <StatusBadge color="success">Success</StatusBadge>
      <StatusBadge color="warning">Warning</StatusBadge>
      <StatusBadge color="error">Error</StatusBadge>
      <StatusBadge color="muted">Muted</StatusBadge>
    </div>
  ),
};

export const Sizes: Story = {
  render: () => (
    <div className="flex items-center gap-3">
      <StatusBadge color="success" size="sm">Small</StatusBadge>
      <StatusBadge color="success" size="md">Medium</StatusBadge>
    </div>
  ),
};

export const WithIndicator: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <StatusBadge color="success" indicator>Active</StatusBadge>
      <StatusBadge color="warning" indicator>Deploying</StatusBadge>
      <StatusBadge color="error" indicator>Error</StatusBadge>
      <StatusBadge color="muted" indicator>Inactive</StatusBadge>
    </div>
  ),
};

export const WithSpinner: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-2">
      <StatusBadge color="success" indicator spinning>Resuming</StatusBadge>
      <StatusBadge color="warning" indicator spinning>Deploying</StatusBadge>
      <StatusBadge color="error" indicator spinning>Pausing</StatusBadge>
      <StatusBadge color="muted" indicator spinning>Undeploying</StatusBadge>
    </div>
  ),
};
