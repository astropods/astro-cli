import type { ReactNode } from "react";
import { FileText } from "lucide-react";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { AgentDetailHeader } from "./AgentDetailHeader";
import type { RecommendedAgent } from "@/components/RecommendedAgents";

export interface AgentDetailContentProps {
  account: string;
  name: string;
  visibility?: string;
  summary?: string;
  categories: string[];
  readme?: string;
  recommendedAgents?: RecommendedAgent[];
  mobileSidebar?: ReactNode;
}

export function AgentDetailContent({
  account,
  name,
  visibility,
  summary,
  categories,
  readme,
  mobileSidebar,
}: AgentDetailContentProps) {
  const readmeContent = readme;

  return (
    <div className="flex-1 min-w-0 p-6 md:p-8">
      <AgentDetailHeader
        account={account}
        name={name}
        visibility={visibility}
        summary={summary}
        categories={categories}
      />

      {/* Sidebar content inlined on mobile */}
      {mobileSidebar && (
        <div className="min-[900px]:hidden mb-8">{mobileSidebar}</div>
      )}

      {/* README */}
      {readmeContent && (
        <section className="mb-8 overflow-hidden rounded-md border border-border-strong bg-surface">
          <div className="flex items-center gap-2 border-b border-border-strong bg-stone-200 px-4 py-2.5 dark:bg-muted/30">
            <FileText className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-[11px] leading-4 font-mono uppercase tracking-[0.14em] text-muted-foreground">
              ReadMe
            </span>
          </div>
          <div className="px-6 py-5">
              <StyledMarkdown className="prose-headings:font-mono prose-p:font-mono prose-li:font-mono prose-a:font-mono prose-strong:font-mono prose-th:font-mono prose-td:font-mono [&>h1:first-child]:mt-0 [&>h2:first-child]:mt-0 [&>h3:first-child]:mt-0">
                {readmeContent}
              </StyledMarkdown>
          </div>
        </section>
      )}
    </div>
  );
}
