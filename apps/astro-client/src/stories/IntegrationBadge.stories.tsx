import type { Meta, StoryObj } from "@storybook/react-vite";

import { IntegrationBadge } from "@/components/IntegrationBadge";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Drive } from "@/components/ui/svgs/drive";

const meta = {
  title: "Components/Integration/IntegrationBadge",
  component: IntegrationBadge,
  argTypes: {
    icon: {
      control: false,
      description: "React node to render as the icon",
      table: {
        type: { summary: "ReactNode" },
        defaultValue: { summary: "-" },
      },
    },
  },
} satisfies Meta<typeof IntegrationBadge>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    name: "Slack",
    icon: <Slack />,
  },
};

export const GitHub: Story = {
  args: {
    name: "GitHub",
    icon: <GithubLight />,
  },
};

export const GoogleDrive: Story = {
  name: "Google Drive",
  args: {
    name: "Google Drive",
    icon: <Drive />,
  },
};

