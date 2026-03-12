import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Puzzle } from "lucide-react";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Slack } from "@/components/ui/svgs/slack";
import { Linear } from "@/components/ui/svgs/linear";
import { Notion } from "@/components/ui/svgs/notion";
import { Drive } from "@/components/ui/svgs/drive";
import { Gmail } from "@/components/ui/svgs/gmail";

type BadgeItem = {
  name: string;
  icon: ReactNode;
};

function formatIntegrationLabel(name: string): string {
  const normalized = name.trim().toLowerCase();
  if (normalized === "github") return "GitHub";
  return name.trim().replace(/[\s-]+/g, "_").toUpperCase();
}

function IntegrationBadgeExtendedPill({ name, icon }: BadgeItem) {
  return (
    <div className="inline-flex items-center gap-2 rounded-md border border-border-strong bg-surface px-3 py-2">
      <span className="flex h-4 w-4 shrink-0 items-center justify-center [&>svg]:size-full">
        {icon}
      </span>
      <span className="text-[12px] font-medium tracking-[0.03em] text-foreground">
        {formatIntegrationLabel(name)}
      </span>
    </div>
  );
}

const commonItems: BadgeItem[] = [
  { name: "GitHub", icon: <GithubLight /> },
  { name: "Slack", icon: <Slack /> },
  { name: "Linear", icon: <Linear /> },
];

const extendedItems: BadgeItem[] = [
  ...commonItems,
  { name: "Notion", icon: <Notion /> },
  { name: "Google Drive", icon: <Drive /> },
  { name: "Gmail", icon: <Gmail /> },
];

const withFallback: BadgeItem[] = [
  ...commonItems,
  { name: "Webhook", icon: <Puzzle className="h-4 w-4 text-muted-foreground" /> },
];

const meta = {
  title: "Features/Integrations/IntegrationBadgeExtended",
  parameters: {
    layout: "padded",
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const ConnectedAppsRow: Story = {
  render: () => (
    <div className="space-y-2.5">
      <h4 className="text-[13px] font-semibold text-foreground">Connected apps</h4>
      <div className="flex flex-wrap gap-2">
        {commonItems.map((item) => (
          <IntegrationBadgeExtendedPill key={item.name} {...item} />
        ))}
      </div>
    </div>
  ),
};

export const ExtendedSet: Story = {
  render: () => (
    <div className="space-y-2.5">
      <h4 className="text-[13px] font-semibold text-foreground">Connected apps</h4>
      <div className="flex flex-wrap gap-2">
        {extendedItems.map((item) => (
          <IntegrationBadgeExtendedPill key={item.name} {...item} />
        ))}
      </div>
    </div>
  ),
};

export const WithFallbackIcon: Story = {
  render: () => (
    <div className="space-y-2.5">
      <h4 className="text-[13px] font-semibold text-foreground">Connected apps</h4>
      <div className="flex flex-wrap gap-2">
        {withFallback.map((item) => (
          <IntegrationBadgeExtendedPill key={item.name} {...item} />
        ))}
      </div>
    </div>
  ),
};
