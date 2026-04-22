import { Database } from "lucide-react";
import { cn } from "@/lib/utils";
import type { KnowledgeStore, KnowledgeBindingInfo } from "@/lib/api";

export interface KnowledgeBindingPickerProps {
  entries: Record<string, { provider?: string; binding?: string }>;
  bindings: Record<string, string>;
  resolvedBindings: Record<string, KnowledgeBindingInfo>;
  onChange: (bindings: Record<string, string>) => void;
  stores: KnowledgeStore[];
}

export function KnowledgeBindingPicker({
  entries,
  bindings,
  resolvedBindings,
  onChange,
  stores,
}: KnowledgeBindingPickerProps) {
  const entryNames = Object.keys(entries).sort();
  if (entryNames.length === 0) return null;

  return (
    <div className="flex flex-col gap-3">
      {entryNames.map((name) => {
        const entry = entries[name];
        const provider = entry.provider ?? resolvedBindings[name]?.provider;
        const compatibleStores = stores.filter(
          (s) => s.provider === provider && s.status === "ready"
        );
        const selectedArn = bindings[name] ?? "";
        const resolved = resolvedBindings[name];

        return (
          <div
            key={name}
            className="flex items-center justify-between gap-4 rounded-lg border border-border bg-surface-secondary px-4 py-3"
          >
            <div className="flex items-center gap-3 min-w-0">
              <Database className="h-4 w-4 shrink-0 text-text-secondary" strokeWidth={1.5} />
              <div className="min-w-0">
                <div className="text-body font-medium truncate">{name}</div>
                {provider && (
                  <div className="text-body-sm text-text-secondary">{provider}</div>
                )}
              </div>
            </div>

            <select
              value={selectedArn}
              onChange={(e) => {
                const next = { ...bindings };
                if (e.target.value === "") {
                  delete next[name];
                } else {
                  next[name] = e.target.value;
                }
                onChange(next);
              }}
              className={cn(
                "rounded-md border border-border bg-surface px-3 py-1.5 text-body-sm",
                "focus:outline-none focus:ring-2 focus:ring-ring",
                "min-w-[200px]"
              )}
            >
              <option value="">Self-hosted</option>
              {compatibleStores.map((store) => (
                <option key={store.arn} value={store.arn}>
                  {store.name} ({store.status})
                </option>
              ))}
            </select>

            {resolved && (
              <span className="text-mono-sm text-text-tertiary shrink-0">
                {resolved.status}
              </span>
            )}
          </div>
        );
      })}
    </div>
  );
}
