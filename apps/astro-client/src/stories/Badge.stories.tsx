import type { Meta, StoryObj } from "@storybook/react-vite";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Gmail } from "@/components/ui/svgs/gmail";

import { Badge } from "@/components/Badge";

const meta = {
  title: "Components/Badge",
  component: Badge,
  argTypes: {
    variant: {
      control: "select",
      options: ["default", "active", "pending", "inactive", "warning", "error", "info"],
    },
    size: {
      control: "select",
      options: ["default", "lg"],
    },
    showDot: { control: "boolean" },
  },
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

export const WithIcon: Story = {
  name: "With Icon",
  args: { icon: <Slack />, children: "Slack" },
};

export const LargeSize: Story = {
  name: "Large Size",
  args: { size: "lg", children: "Google Drive" },
};

export const LargeWithIcon: Story = {
  name: "Large With Icon",
  args: { size: "lg", icon: <Gmail />, children: "Gmail" },
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

export const SizeComparison: Story = {
  name: "Size Comparison",
  args: { children: "Slack" },
  render: () => (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <Badge icon={<Slack />}>Slack</Badge>
        <Badge icon={<GithubLight />}>GitHub</Badge>
        <Badge icon={<Gmail />}>Gmail</Badge>
      </div>
      <div className="flex items-center gap-2">
        <Badge size="lg" icon={<Slack />}>Slack</Badge>
        <Badge size="lg" icon={<GithubLight />}>GitHub</Badge>
        <Badge size="lg" icon={<Gmail />}>Gmail</Badge>
      </div>
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
