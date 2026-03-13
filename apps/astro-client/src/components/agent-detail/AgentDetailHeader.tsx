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
  return (
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
          {categories.length > 0 && (
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {categories.map((tag) => (
                <InlineBadge key={tag}>{tag}</InlineBadge>
              ))}
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
