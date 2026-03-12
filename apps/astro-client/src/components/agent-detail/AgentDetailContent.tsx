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
      {/* Header */}
      <header className="mb-4">
        <div className="flex items-start gap-4">
          <AgentIdentity
            account={account}
            name={name}
            size={56}
            className="size-14 shrink-0 rounded-md overflow-hidden border border-stone-200 dark:border-border"
          />
          <div className="min-w-0">
            <h1 className="flex flex-wrap items-center gap-2 font-mono text-xl font-bold text-foreground">
              {name}
              {visibility === "private" && <PrivacyBadge />}
            </h1>
            <p className="font-mono text-body-sm text-muted-foreground mt-0.5">
              @{account}
              {categories.length > 0 && <> · {categories[0]}</>}
            </p>
          </div>
        </div>
        {/* Category tags */}
        {categories.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-1.5">
            {categories.map((tag) => (
              <InlineBadge key={tag}>{tag}</InlineBadge>
            ))}
          </div>
        )}
      </header>

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
