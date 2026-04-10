import { type ReactNode } from "react";
import { Link } from "react-router";
import { FileText, BookOpen, ArrowUpRight, Slack } from "lucide-react";
import { CommandLineIcon } from "@heroicons/react/24/outline";
import { StyledMarkdown } from "@/components/StyledMarkdown";
import { BlueprintDetailHeader } from "./BlueprintDetailHeader";
import { CopyButton } from "@/components/ui/copy-button";
import { Button } from "@/components/ui/button";

// ─── Shared setup components ─────────────────────────────────────────────────


function CodeBlock({ command, label }: { command: string; label?: string }) {
  return (
    <div>
      {label && <p className="text-muted-foreground mb-1.5 text-xs">{label}</p>}
      <div className="flex items-center justify-between gap-3 rounded-lg border border-stone-200 bg-white px-4 py-3 font-mono text-mono-md text-foreground">
        <code className="overflow-x-auto whitespace-nowrap text-foreground">
          <span className="mr-2">$</span>
          {command}
        </code>
        <CopyButton
          copyText={command}
          className="shrink-0 border-stone-200 bg-white text-stone-500 hover:border-stone-300 hover:bg-stone-100 hover:text-stone-700"
        />
      </div>
    </div>
  );
}

function StepNumber({ n, isLast = false }: { n: number; isLast?: boolean }) {
  return (
    <div className="flex flex-col items-center self-stretch">
      <span className="flex size-7 shrink-0 items-center justify-center rounded-full bg-primary text-[11px] font-bold text-primary-foreground">
        {n}
      </span>
      {!isLast && <div className="w-[2px] flex-1 bg-primary/20 mt-1.5" />}
    </div>
  );
}

// ─── Component ───────────────────────────────────────────────────────────────

export interface BlueprintDetailContentProps {
  account: string;
  name: string;
  categories: string[];
  canEdit?: boolean;
  readme?: string;
  mobileSidebar?: ReactNode;
  isDraft?: boolean;
}

export function BlueprintDetailContent({
  account,
  name,
  categories,
  canEdit,
  readme,
  mobileSidebar,
  isDraft = false,
}: BlueprintDetailContentProps) {
  return (
    <div className="flex-1 min-w-0 p-6 md:p-8">
      <BlueprintDetailHeader
        account={account}
        name={name}
        categories={categories}
        canEdit={canEdit}
        isDraft={isDraft}
      />

      {/* Sidebar content inlined on mobile */}
      {mobileSidebar && (
        <div className="min-[900px]:hidden mb-8">{mobileSidebar}</div>
      )}

      {/* Draft: FINISH SETTING UP */}
      {isDraft && (
        <section className="mb-8 overflow-hidden rounded-md border border-border-strong bg-surface">
          <div className="flex items-center justify-between gap-4 border-b border-border-strong bg-stone-200 px-4 py-2.5 dark:bg-muted/30">
            <div className="flex items-center gap-2">
              <CommandLineIcon className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="text-[11px] leading-4 font-mono uppercase tracking-[0.14em] text-muted-foreground">
                Finish setup
              </span>
            </div>
            <div className="flex items-center gap-4">
              <Link
                to="https://github.com/astropods/agents"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
              >
                <ArrowUpRight className="h-3 w-3" />
                View examples
              </Link>
              <Link
                to="https://docs.astropods.com/astropods-package-spec"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
              >
                <ArrowUpRight className="h-3 w-3" />
                View package spec
              </Link>
            </div>
          </div>

          <div className="px-6 py-6">
            <div className="space-y-0">
              <section className="flex gap-4">
                <StepNumber n={1} />
                <div className="flex-1 pb-8">
                  <h3 className="text-sm font-semibold mb-3 pt-1">Install the Astro CLI</h3>
                  <CodeBlock command="curl -fsSL https://astropods.ai/install | sh" />
                </div>
              </section>
              <section className="flex gap-4">
                <StepNumber n={2} />
                <div className="flex-1 pb-8">
                  <h3 className="text-sm font-semibold mb-3 pt-1">Scaffold & configure your agent</h3>
                  <div className="space-y-3">
                    <CodeBlock command={`ast create ${name}`} />
                    <CodeBlock command={`cd ${name} && ast dev`} />
                  </div>
                  <p className="text-muted-foreground mt-2 text-xs">
                    This creates your project locally. Fill in your{" "}
                    <Link to="https://docs.astropods.com/agent-card-spec" target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-0.5 font-mono text-foreground hover:text-teal-600 transition-colors">
                      agent.md<ArrowUpRight className="size-3" />
                    </Link>{" "}and
                    configure your agent before pushing.
                  </p>
                </div>
              </section>
              <section className="flex gap-4">
                <StepNumber n={3} isLast />
                <div className="flex-1 pb-2">
                  <h3 className="text-sm font-semibold mb-3 pt-1">Push to the registry</h3>
                  <div className="space-y-3">
                    <CodeBlock command="ast login" />
                    <CodeBlock command="ast push" />
                  </div>
                  <p className="text-muted-foreground mt-2 text-xs">
                    Use <span className="font-mono text-foreground">ast push --build</span>{" "}
                    to force a rebuild before pushing.
                  </p>
                </div>
              </section>
            </div>
          </div>

          {/* Need more support? */}
          <div className="flex items-center justify-between gap-6 border-t border-border-strong bg-stone-50 px-6 py-4 dark:bg-muted/20">
            <div>
              <p className="text-sm font-semibold">Need more support?</p>
              <p className="text-muted-foreground text-xs">Resources to help you build and deploy your agent.</p>
            </div>
            <div className="flex shrink-0 gap-3">
              <Button variant="outline" size="sm" className="gap-2" asChild>
                <Link to="https://docs.astropods.com/welcome" target="_blank" rel="noopener noreferrer">
                  <BookOpen className="size-4" strokeWidth={1.75} />
                  View docs
                </Link>
              </Button>
              <Button variant="outline" size="sm" className="gap-2" asChild>
                <Link to="https://join.slack.com/t/astropods-ai/shared_invite/zt-3v03e93dw-mPp~0ZxZfcmkexKGv_cofQ" target="_blank" rel="noopener noreferrer">
                  <Slack className="size-4" strokeWidth={1.75} />
                  Join Slack
                </Link>
              </Button>
            </div>
          </div>
        </section>
      )}

      {/* README */}
      {!isDraft && readme && (
        <section className="mb-8 overflow-hidden rounded-md border border-border-strong bg-surface">
          <div className="flex items-center gap-2 border-b border-border-strong bg-stone-200 px-4 py-2.5 dark:bg-muted/30">
            <FileText className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-[11px] leading-4 font-mono uppercase tracking-[0.14em] text-muted-foreground">
              ReadMe
            </span>
          </div>
          <div className="px-6 py-5">
            <StyledMarkdown className="[&>h1:first-child]:mt-0 [&>h2:first-child]:mt-0 [&>h3:first-child]:mt-0">
              {readme}
            </StyledMarkdown>
          </div>
        </section>
      )}
    </div>
  );
}
