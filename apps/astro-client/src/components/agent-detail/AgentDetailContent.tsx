import type { ReactNode } from "react";
import { ShieldCheck } from "lucide-react";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { InlineBadge } from "@/components/InlineBadge";
import { AgentIdentity } from "@/components/AgentIdentity";
import { RecommendedAgents } from "@/components/RecommendedAgents";
import type { RecommendedAgent } from "@/components/RecommendedAgents";

export interface AgentDetailContentProps {
  account: string;
  name: string;
  categories: string[];
  readme?: string;
  safetyPermissions: string[];
  recommendedAgents: RecommendedAgent[];
  mobileSidebar?: ReactNode;
}

export function AgentDetailContent({
  account,
  name,
  categories,
  readme,
  safetyPermissions,
  recommendedAgents,
  mobileSidebar,
}: AgentDetailContentProps) {
  return (
    <div className="flex-1 min-w-0 p-6 md:p-8 min-[900px]:max-w-3xl">
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
            <h1 className="font-mono text-lg font-bold text-foreground">
              {name}
            </h1>
            <p className="font-mono text-[11px] text-muted-foreground mt-0.5">
              by {account}
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

      {/* Safety & Permissions */}
      {safetyPermissions.length > 0 && (
        <section className="mb-8">
          <h2 className="text-[15px] font-bold text-foreground mb-3.5 pb-2.5 border-b border-border-strong">
            Safety & Permissions
          </h2>
          <ul className="space-y-2.5">
            {safetyPermissions.map((permission, i) => (
              <li key={i} className="flex items-start gap-2.5 text-[13px] leading-relaxed">
                <ShieldCheck className="size-3.5 shrink-0 text-muted-foreground mt-0.5" />
                <span className="text-muted-foreground">{permission}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <RecommendedAgents agents={recommendedAgents} />
    </div>
  );
}
