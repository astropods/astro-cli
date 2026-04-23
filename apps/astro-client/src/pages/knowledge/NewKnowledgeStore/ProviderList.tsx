import { Tag } from "@/components/Tag";
import { PROVIDER_LABELS, ALL_PROVIDERS, MANAGED_SET, PROVIDER_CATEGORIES } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeProvider } from "@/lib/api";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";

export function ProviderList({ onSelect }: { onSelect: (p: KnowledgeProvider) => void }) {
  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="text-heading-1 text-foreground">Choose a provider</h2>
      <p className="mt-1 text-body-sm text-muted-foreground">
        Pick the database or vector store to back this knowledge store.
      </p>

      <div className="mt-6 space-y-3">
        {ALL_PROVIDERS.map((p) => (
          <button
            key={p}
            type="button"
            onClick={() => onSelect(p)}
            className="flex w-full cursor-pointer items-center gap-4 rounded-lg border border-border bg-white px-5 py-4 text-left transition-all hover:border-teal-500 hover:shadow-md dark:bg-popover dark:hover:border-teal-700"
          >
            <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
              <ProviderIcon provider={p} className="size-6" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="font-medium text-foreground">{PROVIDER_LABELS[p]}</span>
                {MANAGED_SET.has(p) && <Tag color="blue">Managed option</Tag>}
              </div>
              <p className="text-body-sm text-muted-foreground">{PROVIDER_CATEGORIES[p]}</p>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
