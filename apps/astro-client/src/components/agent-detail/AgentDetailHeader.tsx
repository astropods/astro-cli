import { PrivacyBadge } from "@/components/PrivacyBadge";
import { InlineBadge } from "@/components/InlineBadge";
import { AgentIdentity } from "@/components/AgentIdentity";

export interface AgentDetailHeaderProps {
  account: string;
  name: string;
  visibility?: string;
  summary?: string;
  categories: string[];
}

export function AgentDetailHeader({
  account,
  name,
  visibility,
  summary,
  categories,
}: AgentDetailHeaderProps) {
  return (
    <header className="mb-6 border-b border-border-strong pb-6">
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
          </p>
        </div>
      </div>
      {summary && (
        <p className="mt-4 max-w-4xl text-[14px] leading-[1.65] text-foreground/85">
          {summary}
        </p>
      )}
      {categories.length > 0 && (
        <div className="mt-3.5 flex flex-wrap gap-2">
          {categories.map((tag) => (
            <InlineBadge
              key={tag}
              className="rounded-[4px] bg-surface text-muted-foreground dark:bg-surface dark:text-muted-foreground"
            >
              {tag}
            </InlineBadge>
          ))}
        </div>
      )}
    </header>
  );
}
