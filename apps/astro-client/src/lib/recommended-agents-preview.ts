import type { AgentCardProps } from "@/components/AgentCard";

// Temporary hardcoded recommended agents for UI styling previews.
// Remove this once backend recommendation data is available everywhere.
export const recommendedAgentsPreview: AgentCardProps[] = [
  {
    slug: "acme/incident-briefing-bot",
    account: "acme",
    name: "incident-briefing-bot",
    description: "Summarizes incidents and drafts stakeholder updates.",
  },
  {
    slug: "acme/oncall-optimizer",
    account: "acme",
    name: "oncall-optimizer",
    description: "Prioritizes alerts and routes handoffs to the right owner.",
  },
];
