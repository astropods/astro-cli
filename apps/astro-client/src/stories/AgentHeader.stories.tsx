import type { Meta, StoryObj } from "@storybook/react-vite";

import { PauseIcon } from "@heroicons/react/24/solid";
import { AgentHeader } from "@/components/AgentHeader";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Linear } from "@/components/ui/svgs/linear";
import { Notion } from "@/components/ui/svgs/notion";
import { Drive } from "@/components/ui/svgs/drive";

const meta = {
  title: "Features/Agents/AgentHeader",
  component: AgentHeader,
  argTypes: {
    onMenuClick: { table: { disable: true } },
  },
  args: {
    onMenuClick: () => {},
  },
} satisfies Meta<typeof AgentHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Active: Story = {
  args: {
    name: "Customer Insights Engine",

    status: "active",
    integrations: [
      { name: "Slack", icon: <Slack /> },
      { name: "GitHub", icon: <GithubLight /> },
    ],
    primaryAction: { label: "Pause", icon: <PauseIcon className="size-4" />, onClick: () => {} },
  },
};

export const Pending: Story = {
  args: {
    name: "Incident Commander",

    status: "pending",
    integrations: [{ name: "Linear", icon: <Linear /> }],
    primaryAction: { label: "Deploy", onClick: () => {} },
  },
};

export const Inactive: Story = {
  args: {
    name: "Legacy Bot",

    status: "inactive",
  },
};

export const NoAvatar: Story = {
  name: "No Avatar",
  args: {
    name: "Data Pipeline Agent",
    status: "active",
    integrations: [{ name: "Notion", icon: <Notion /> }],
    primaryAction: { label: "Open", onClick: () => {} },
  },
};

export const ManyIntegrations: Story = {
  name: "Many Integrations",
  args: {
    name: "Super Agent",

    status: "active",
    integrations: [
      { name: "Slack", icon: <Slack /> },
      { name: "GitHub", icon: <GithubLight /> },
      { name: "Linear", icon: <Linear /> },
      { name: "Notion", icon: <Notion /> },
      { name: "Google Drive", icon: <Drive /> },
    ],
    primaryAction: { label: "Manage", onClick: () => {} },
  },
};
