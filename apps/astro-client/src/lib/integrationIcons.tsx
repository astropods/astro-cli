import type { ReactNode } from "react";
import type { IntegrationIconStackItem } from "@/components/IntegrationIconStack";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Linear } from "@/components/ui/svgs/linear";
import { Notion } from "@/components/ui/svgs/notion";
import { Drive } from "@/components/ui/svgs/drive";
import { Gmail } from "@/components/ui/svgs/gmail";

export const integrationIconMap: Record<string, ReactNode> = {
  Slack: <Slack />,
  GitHub: <GithubLight />,
  Linear: <Linear />,
  Notion: <Notion />,
  "Google Drive": <Drive />,
  "Google Docs": <Drive />,
  Drive: <Drive />,
  Gmail: <Gmail />,
};

export function getIntegrationItems(
  names: string[],
): IntegrationIconStackItem[] {
  return names
    .filter((name) => name in integrationIconMap)
    .map((name) => ({ name, icon: integrationIconMap[name] }));
}
