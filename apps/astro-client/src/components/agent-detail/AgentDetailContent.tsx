import type { ReactNode } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { ShieldCheck } from "lucide-react";
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
    <div className="flex-1 min-w-0 p-6 md:p-8 lg:max-w-3xl">
      {/* Header */}
      <header className="mb-8">
        <div className="flex items-center gap-3">
          <AgentIdentity
            account={account}
            name={name}
            size={40}
            className="size-10 shrink-0 rounded-sm overflow-hidden"
          />
          <h1 className="text-2xl leading-tight font-semibold text-foreground">
            {name}
          </h1>
        </div>
        <hr className="mt-4 border-border" />

        {/* Category tags */}
        {categories.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-2">
            {categories.map((tag) => (
              <InlineBadge key={tag}>{tag}</InlineBadge>
            ))}
          </div>
        )}
      </header>

      {/* Sidebar content inlined on mobile */}
      {mobileSidebar && (
        <div className="lg:hidden mb-8">{mobileSidebar}</div>
      )}

      {/* README */}
      {readme && (
        <section className="mb-8">
          <div className="prose prose-stone dark:prose-invert max-w-none overflow-x-auto prose-headings:font-bold prose-headings:text-foreground text-foreground prose-p:my-2 prose-headings:mt-6 prose-headings:mb-2 prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5 prose-pre:my-3 prose-blockquote:my-3 prose-hr:my-4 [&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-stone-100 [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-foreground [&_:not(pre)>code]:font-normal [&_:not(pre)>code]:before:content-[''] [&_:not(pre)>code]:after:content-['']">
            <Markdown remarkPlugins={[remarkGfm]}>{readme}</Markdown>
          </div>
        </section>
      )}

      {/* Safety & Permissions */}
      {safetyPermissions.length > 0 && (
        <section className="mb-8">
          <h2 className="text-xs font-medium text-muted-foreground mb-3">
            Safety & Permissions
          </h2>
          <ul className="space-y-2">
            {safetyPermissions.map((permission, i) => (
              <li key={i} className="flex items-start gap-2 text-sm">
                <ShieldCheck className="size-4 shrink-0 text-muted-foreground mt-0.5" />
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
