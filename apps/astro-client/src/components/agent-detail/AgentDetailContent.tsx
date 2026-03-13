import type { ReactNode } from "react";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { RecommendedAgents } from "@/components/RecommendedAgents";
import type { RecommendedAgent } from "@/components/RecommendedAgents";
import { AgentDetailHeader } from "./AgentDetailHeader";

export interface AgentDetailContentProps {
  account: string;
  name: string;
  visibility?: string;
  categories: string[];
  readme?: string;
  recommendedAgents: RecommendedAgent[];
  mobileSidebar?: ReactNode;
}

export function AgentDetailContent({
  account,
  name,
  visibility,
  categories,
  readme,
  recommendedAgents,
  mobileSidebar,
}: AgentDetailContentProps) {
  return (
    <div className="flex-1 min-w-0 p-6 md:p-8">
      <AgentDetailHeader
        account={account}
        name={name}
        visibility={visibility}
        categories={categories}
      />

      {/* Sidebar content inlined on mobile */}
      {mobileSidebar && (
        <div className="min-[900px]:hidden mb-8">{mobileSidebar}</div>
      )}

      {/* README */}
      {readme && (
        <section className="mb-8">
          <StyledMarkdown>{readme}</StyledMarkdown>
        </section>
      )}

      <RecommendedAgents agents={recommendedAgents} />
    </div>
  );
}
