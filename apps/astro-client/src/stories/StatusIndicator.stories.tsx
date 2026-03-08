import type { Meta, StoryObj } from "@storybook/react-vite";
import { StatusIndicator } from "@/components/StatusIndicator";

const meta = {
  title: "Design System/Primitives/StatusIndicator",
  component: StatusIndicator,
  argTypes: {
    variant: {
      control: "select",
      options: ["success", "pending", "muted", "warning", "error"],
    },
    pulse: { control: "boolean" },
  },
} satisfies Meta<typeof StatusIndicator>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Live: Story = {
  args: { variant: "success", children: "Live" },
};

export const Deploying: Story = {
  args: { variant: "pending", pulse: true, children: "Deploying" },
};

export const Warning: Story = {
  args: { variant: "warning", children: "Issues Found" },
};

export const Error: Story = {
  args: { variant: "error", children: "Error" },
};

export const Inactive: Story = {
  args: { variant: "muted", children: "Inactive" },
};

export const AllVariants: Story = {
  args: { children: "Live" },
  render: () => (
    <div className="flex flex-col gap-3">
      <StatusIndicator variant="success">Live</StatusIndicator>
      <StatusIndicator variant="pending" pulse>Deploying</StatusIndicator>
      <StatusIndicator variant="warning">Issues Found</StatusIndicator>
      <StatusIndicator variant="error">Error</StatusIndicator>
      <StatusIndicator variant="muted">Inactive</StatusIndicator>
    </div>
  ),
};
