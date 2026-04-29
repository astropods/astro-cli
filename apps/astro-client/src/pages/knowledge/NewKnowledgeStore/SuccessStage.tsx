import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Tag } from "@/components/Tag";
import { StatusBadge } from "@/components/StatusBadge";
import { CopyButton } from "@/components/ui/copy-button";
import { LiveRevealConfetti } from "@/components/deployed-agent/detail/LiveRevealConfetti";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeStore } from "@/lib/api";

export function SuccessStage({ store }: { store: KnowledgeStore }) {
  const modeLabel = store.mode === "managed" ? "Managed" : "External";
  const yamlSnippet = `knowledge:\n  - store: ${store.name}\n    as: ${store.name.split("-")[0]}`;
  const cliCommand = `ast dev --source ${store.name}`;

  return (
    <div className="mx-auto max-w-lg">
      <div className="fixed inset-0 pointer-events-none z-0">
        <LiveRevealConfetti />
      </div>

      <div className="flex flex-col items-center text-center mb-9 gap-3.5">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-full border-[1.5px] border-teal-600/25 dark:border-teal-400/25 bg-teal-600/10 dark:bg-teal-400/10 [animation:ks-pop_0.5s_cubic-bezier(0.34,1.56,0.64,1)_both]">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
            <path
              d="M4.5 10.5l4 4 7-8"
              strokeWidth="1.8"
              strokeLinecap="round"
              strokeLinejoin="round"
              pathLength="1"
              strokeDasharray="1"
              className="stroke-teal-700 dark:stroke-teal-400 [stroke-dashoffset:1] [animation:ks-check-draw_0.6s_ease-out_0.3s_both]"
            />
          </svg>
        </div>
        <div className="flex flex-col gap-1.5">
          <h2 className="text-heading-1 text-foreground">Store added</h2>
          <p className="text-body text-muted-foreground">
            {store.mode === "managed"
              ? "Your managed store is ready. Bind it to an agent to start using it."
              : "Your store is connected. Bind it to an agent to start using it."}
          </p>
        </div>
      </div>

      <div className="space-y-3 mb-7">
        <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface">
          <div className="flex items-center gap-3 px-4 py-4">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
              <ProviderIcon provider={store.provider} className="size-5" />
            </div>
            <div className="flex-1 min-w-0">
              <span className="font-medium text-foreground">{store.name}</span>
              <p className="mt-0.5 text-body-sm text-muted-foreground">{PROVIDER_LABELS[store.provider]}</p>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <Tag color={store.mode === "managed" ? "blue" : "default"}>{modeLabel}</Tag>
              <StatusBadge color="success" indicator>Ready</StatusBadge>
            </div>
          </div>
        </div>

        <div className="rounded-lg overflow-hidden border border-border bg-white dark:bg-surface">
          <div className="flex flex-col gap-2.5 px-5 pt-4 pb-3.5 border-b border-border/60">
            <p className="font-medium text-body text-foreground">Use in your agent</p>
            <div className="flex items-center justify-between gap-3 rounded-sm bg-stone-200 dark:bg-muted px-4 py-2 font-mono text-mono-sm text-foreground">
              <div className="flex-1 min-w-0 overflow-x-auto [scrollbar-width:thin] [scrollbar-color:theme(colors.stone.400)_transparent] [&::-webkit-scrollbar]:h-1 [&::-webkit-scrollbar-track]:bg-transparent [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-stone-400 dark:[&::-webkit-scrollbar-thumb]:bg-muted-foreground">
                <pre className="whitespace-pre leading-relaxed text-foreground">{yamlSnippet}</pre>
              </div>
              <CopyButton copyText={yamlSnippet} className="shrink-0 border-stone-200 dark:border-muted bg-stone-200 dark:bg-muted hover:bg-stone-200 dark:hover:bg-muted text-stone-500 dark:text-muted-foreground hover:text-stone-700 dark:hover:text-foreground" />
            </div>
          </div>
          <div className="flex flex-col gap-2.5 px-5 pt-4 pb-3.5">
            <p className="font-medium text-body text-foreground">CLI shortcut</p>
            <div className="flex items-center justify-between gap-3 rounded-sm bg-stone-200 dark:bg-muted px-4 py-2 font-mono text-mono-sm text-foreground">
              <div className="flex-1 min-w-0 overflow-x-auto [scrollbar-width:thin] [scrollbar-color:theme(colors.stone.400)_transparent] [&::-webkit-scrollbar]:h-1 [&::-webkit-scrollbar-track]:bg-transparent [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-stone-400 dark:[&::-webkit-scrollbar-thumb]:bg-muted-foreground">
                <code className="whitespace-nowrap text-foreground">
                  <span className="mr-2 text-muted-foreground">$</span>{cliCommand}
                </code>
              </div>
              <CopyButton copyText={cliCommand} className="shrink-0 border-stone-200 dark:border-muted bg-stone-200 dark:bg-muted hover:bg-stone-200 dark:hover:bg-muted text-stone-500 dark:text-muted-foreground hover:text-stone-700 dark:hover:text-foreground" />
            </div>
          </div>
        </div>
      </div>

      <div className="flex items-center justify-between">
        <Button variant="ghost" className="pl-0" asChild>
          <Link to={knowledgePath}>&larr; Back to stores</Link>
        </Button>
        <Button asChild>
          <Link to={knowledgeDetailPath(store.name)}>View store &rarr;</Link>
        </Button>
      </div>
    </div>
  );
}
