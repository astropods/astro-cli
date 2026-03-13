import type { ReactNode } from "react";
import type { IntegrationIconStackItem } from "@/components/IntegrationIconStack";
import type { ResolvedIntegration } from "@/lib/api";
import { Slack } from "@/components/ui/svgs/slack";
import { GithubLight } from "@/components/ui/svgs/githubLight";
import { Linear } from "@/components/ui/svgs/linear";
import { Notion } from "@/components/ui/svgs/notion";
import { Drive } from "@/components/ui/svgs/drive";
import { Gmail } from "@/components/ui/svgs/gmail";

// Keyed by canonical integration ID
export const integrationIconMap: Record<string, ReactNode> = {
  slack: <Slack />,
  github: <GithubLight />,
  linear: <Linear />,
  notion: <Notion />,
  "google-drive": <Drive />,
  "google-sheets": <Drive />,
  gmail: <Gmail />,
};

export function getIntegrationItems(
  integrations: ResolvedIntegration[],
): IntegrationIconStackItem[] {
  return integrations
    .filter((i) => i.id in integrationIconMap)
    .map((i) => ({ name: i.name, icon: integrationIconMap[i.id] }));
}
