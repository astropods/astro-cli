import { Link } from "react-router";
import { CheckIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import { knowledgePath, knowledgeDetailPath } from "@/lib/routes";
import type { KnowledgeStore } from "@/lib/api";
import { CopyButton } from "@/components/ui/copy-button";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

export function SuccessStage({ store }: { store: KnowledgeStore }) {
  const modeLabel = store.mode === "managed" ? "Managed" : "External";
  const yamlSnippet = `knowledge:\n  - store: ${store.name}\n    as: ${store.name.split("-")[0]}`;
  const cliCommand = `ast dev --source ${store.name}`;

  return (
    <div className="mx-auto max-w-lg">
      <div className="flex flex-col items-center text-center">
        <div className="flex size-12 items-center justify-center rounded-full border-2 border-teal-200 bg-teal-50">
          <CheckIcon className="size-6 text-teal-600" />
        </div>
        <h2 className="mt-4 text-heading-1 text-foreground">Store added</h2>
        <p className="mt-1 text-body-sm text-muted-foreground">
          {store.mode === "managed"
            ? "Your managed store is ready. Bind it to an agent to start using it."
            : "Your store is connected. Bind it to an agent to start using it."}
        </p>
      </div>

      <div className="mt-8 space-y-4">
        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-center gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted">
              <ProviderIcon provider={store.provider} className="size-6" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-medium text-foreground">{store.name}</span>
                <span className="inline-flex items-center gap-1 text-mono-sm text-teal-700">
                  <span className="size-1.5 rounded-full bg-teal-600" />
                  Ready
                </span>
              </div>
              <p className="text-body-sm text-muted-foreground">
                {PROVIDER_LABELS[store.provider]} &middot; {modeLabel}
              </p>
            </div>
          </div>

          <div className="mt-4 border-t border-border pt-4">
            <p className="font-mono text-mono-sm uppercase tracking-wide text-muted-foreground">
              Astro Resource Name
            </p>
            <div className="mt-1 flex items-center justify-between gap-2">
              <code className="text-body-sm text-foreground">{store.arn}</code>
              <CopyButton copyText={store.arn} />
            </div>
          </div>
        </div>

        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-medium text-foreground">Use in your agent</p>
              <p className="text-body-sm text-muted-foreground">
                Add this to your astropods.yml to give an agent access.
              </p>
            </div>
            <CopyButton copyText={yamlSnippet} />
          </div>
          <pre className="mt-3 whitespace-pre rounded-md bg-muted px-4 py-3 font-mono text-mono-sm text-foreground">
            {yamlSnippet}
          </pre>
        </div>

        <div className="rounded-lg border border-border bg-surface p-5">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-medium text-foreground">CLI shortcut</p>
              <p className="text-body-sm text-muted-foreground">Use in local development</p>
            </div>
            <CopyButton copyText={cliCommand} />
          </div>
          <code className="mt-3 block font-mono text-body-sm text-foreground">{cliCommand}</code>
        </div>
      </div>

      <div className="mt-8">
        <Button size="lg" className="w-full" asChild>
          <Link to={knowledgeDetailPath(store.name)}>
            View store &rarr;
          </Link>
        </Button>
        <div className="mt-3 text-center">
          <Link
            to={knowledgePath}
            className="text-body-sm text-muted-foreground hover:text-foreground transition-colors"
          >
            Back to Knowledge Stores
          </Link>
        </div>
      </div>
    </div>
  );
}
