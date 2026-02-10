import type { Meta, StoryObj } from "@storybook/react-vite";

import { IntegrationBadge } from "@/components/IntegrationBadge";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Linear } from "@/components/ui/svgs/linear";
import { Notion } from "@/components/ui/svgs/notion";
import { Drive } from "@/components/ui/svgs/drive";

const meta = {
  title: "Components/IntegrationBadge",
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

export const OverlappingGroup: Story = {
  name: "Overlapping Group",
  render: () => (
    <div className="flex items-center">
      <IntegrationBadge name="Slack" icon={<Slack />} />
      <IntegrationBadge name="GitHub" icon={<GithubLight />} className="-ml-1" />
      <IntegrationBadge name="Linear" icon={<Linear />} className="-ml-1" />
      <IntegrationBadge name="Notion" icon={<Notion />} className="-ml-1" />
      <IntegrationBadge name="Google Drive" icon={<Drive />} className="-ml-1" />
    </div>
  ),
};
