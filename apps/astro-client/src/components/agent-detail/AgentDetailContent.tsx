import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { ShieldCheck } from "lucide-react";
import { Badge } from "@/components/Badge";
import { RecommendedAgents } from "@/components/RecommendedAgents";
import type { RecommendedAgent } from "@/components/RecommendedAgents";

export interface AgentDetailContentProps {
  account: string;
  name: string;
  description: string;
  categories: string[];
  readme?: string;
  safetyPermissions: string[];
  recommendedAgents: RecommendedAgent[];
}

export function AgentDetailContent({
  account,
  name,
  description,
  categories,
  readme,
  safetyPermissions,
  recommendedAgents,
}: AgentDetailContentProps) {
  return (
    <div className="flex-1 min-w-0 p-6 md:p-8 max-w-3xl">
      {/* Header */}
      <header className="mb-8">
        <h1 className="text-[32px] leading-tight font-semibold text-foreground">
          <span className="font-normal text-stone-400">{account}/</span>
          {name}
        </h1>
        <p className="mt-2 text-sm text-stone-600 leading-relaxed">
          {description}
        </p>

        {/* Category tags */}
        {categories.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-2">
            {categories.map((tag) => (
              <Badge key={tag}>{tag}</Badge>
            ))}
          </div>
        )}
      </header>

      {/* README */}
      {readme && (
        <section className="mb-8">
          <div className="prose prose-stone dark:prose-invert max-w-none prose-headings:font-bold prose-headings:text-foreground text-stone-500 prose-p:my-2 prose-headings:mt-6 prose-headings:mb-2 prose-ul:my-2 prose-ol:my-2 prose-li:my-0.5 prose-pre:my-3 prose-blockquote:my-3 prose-hr:my-4 [&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-stone-100 [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-stone-700 [&_:not(pre)>code]:font-normal [&_:not(pre)>code]:before:content-[''] [&_:not(pre)>code]:after:content-['']">
            <Markdown remarkPlugins={[remarkGfm]}>{readme}</Markdown>
          </div>
        </section>
      )}

      {/* Safety & Permissions */}
      {safetyPermissions.length > 0 && (
        <section className="mb-8">
          <h2 className="text-xs font-medium text-stone-400 mb-3">
            Safety & Permissions
          </h2>
          <ul className="space-y-2">
            {safetyPermissions.map((permission, i) => (
              <li key={i} className="flex items-start gap-2 text-sm">
                <ShieldCheck className="size-4 shrink-0 text-stone-400 mt-0.5" />
                <span className="text-stone-600">{permission}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <RecommendedAgents agents={recommendedAgents} />
    </div>
  );
}
