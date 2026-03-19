import { PrivacyBadge } from "@/components/PrivacyBadge";
import { InlineBadge } from "@/components/InlineBadge";
import { AgentIdentity } from "@/components/AgentIdentity";

export interface AgentDetailHeaderProps {
  account: string;
  name: string;
  visibility?: string;
  categories: string[];
}

export function AgentDetailHeader({
  account,
  name,
  visibility,
  categories,
}: AgentDetailHeaderProps) {
  const hasCategories = categories.length > 0;

  return (
    <header className="mb-6 border-b border-border-strong pb-6">
      <div className={`flex gap-4 ${hasCategories ? "items-start" : "items-center"}`}>
        <AgentIdentity
          account={account}
          name={name}
          size={56}
          className="size-14 shrink-0 rounded-sm overflow-hidden border border-stone-200 dark:border-border"
        />
        <div className="min-w-0">
          <h1 className="flex flex-wrap items-center gap-2 font-mono text-xl font-bold text-foreground">
            {name}
            {visibility === "private" && <PrivacyBadge />}
          </h1>
          {hasCategories && (
            <div className="mt-2 flex flex-wrap gap-2">
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
        </div>
      </div>
    </header>
  );
}
