import type { ReactNode } from "react";
import { FileText } from "lucide-react";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { BlueprintDetailHeader } from "./BlueprintDetailHeader";
import { GitHubConnectionPanel } from "./GitHubConnectionPanel";

export interface BlueprintDetailContentProps {
  account: string;
  name: string;
  visibility?: string;
  categories: string[];
  avatarUrl?: string;
  canEdit?: boolean;
  readme?: string;
  mobileSidebar?: ReactNode;
}

export function BlueprintDetailContent({
  account,
  name,
  visibility,
  categories,
  avatarUrl,
  canEdit,
  readme,
  mobileSidebar,
}: BlueprintDetailContentProps) {
  const readmeContent = readme;

  return (
    <div className="flex-1 min-w-0 p-6 md:p-8">
      <BlueprintDetailHeader
        account={account}
        name={name}
        visibility={visibility}
        categories={categories}
        avatarUrl={avatarUrl}
        canEdit={canEdit}
      />

      {/* Sidebar content inlined on mobile */}
      {mobileSidebar && (
        <div className="min-[900px]:hidden mb-8">{mobileSidebar}</div>
      )}

      {/* GitHub connection — only visible to the owner */}
      {canEdit && (
        <div className="mb-8">
          <GitHubConnectionPanel account={account} name={name} />
        </div>
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
              <StyledMarkdown className="[&>h1:first-child]:mt-0 [&>h2:first-child]:mt-0 [&>h3:first-child]:mt-0">
                {readmeContent}
              </StyledMarkdown>
          </div>
        </section>
      )}
    </div>
  );
}
