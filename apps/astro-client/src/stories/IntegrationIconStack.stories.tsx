import type { Meta, StoryObj } from "@storybook/react-vite";

import { IntegrationIconStack } from "@/components/IntegrationIconStack";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Linear } from "@/components/ui/svgs/linear";
import { Notion } from "@/components/ui/svgs/notion";
import { Drive } from "@/components/ui/svgs/drive";

const allIntegrations = [
  { name: "Slack", icon: <Slack /> },
  { name: "GitHub", icon: <GithubLight /> },
  { name: "Linear", icon: <Linear /> },
  { name: "Notion", icon: <Notion /> },
  { name: "Google Drive", icon: <Drive /> },
];

const meta = {
  title: "Components/Integration/IntegrationIconStack",
  component: IntegrationIconStack,
} satisfies Meta<typeof IntegrationIconStack>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    integrations: allIntegrations,
  },
};

export const WithOverflow: Story = {
  name: "With Overflow (+N)",
  args: {
    integrations: allIntegrations,
    max: 3,
  },
};

export const NoOverflow: Story = {
  name: "No Overflow",
  args: {
    integrations: allIntegrations.slice(0, 3),
    max: 3,
  },
};

export const SingleIcon: Story = {
  name: "Single Icon",
  args: {
    integrations: [allIntegrations[0]],
  },
};
