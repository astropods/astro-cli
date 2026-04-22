import { Package, CloudCog } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeStore, KnowledgeBindingInfo, KnowledgeProvider } from "@/lib/api";

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
    <div className="space-y-2">
      {entryNames.map((name) => {
        const entry = entries[name];
        const provider = (entry.provider ?? resolvedBindings[name]?.provider) as KnowledgeProvider | undefined;
        const providerLabel = provider ? PROVIDER_LABELS[provider] ?? provider : undefined;
        const compatibleStores = stores.filter(
          (s) => s.provider === provider && s.status === "ready"
        );
        const rawArn = bindings[name] ?? "";
        const isBound = rawArn !== "";
        const selectValue = isBound ? rawArn : "__builtin__";
        const resolved = resolvedBindings[name];

        return (
          <div
            key={name}
            className={cn(
              "rounded-[6px] border transition-[border-color,background-color]",
              isBound
                ? "border-primary/40 bg-primary/5"
                : "border-border bg-transparent",
            )}
          >
            <div className="flex items-center gap-4 px-4 py-3">
              {/* Provider icon + entry name */}
              <div className={cn(
                "flex h-9 w-9 items-center justify-center rounded-sm shrink-0 transition-colors",
                isBound ? "bg-primary/10" : "bg-stone-200",
              )}>
                {provider ? (
                  <ProviderIcon provider={provider} className="size-5" />
                ) : (
                  <Package className="size-5 text-muted-foreground" strokeWidth={1.5} />
                )}
              </div>

              <div className="flex flex-col gap-0.5 flex-1 min-w-0">
                <span className="text-[13px] font-medium text-foreground truncate">{name}</span>
                {providerLabel && (
                  <span className="text-[11px] text-muted-foreground">{providerLabel}</span>
                )}
              </div>

              {/* Store selector */}
              <div className="shrink-0 w-[220px]">
                <Select
                  value={selectValue}
                  onValueChange={(value) => {
                    const next = { ...bindings };
                    if (value === "__builtin__") {
                      delete next[name];
                    } else {
                      next[name] = value;
                    }
                    onChange(next);
                  }}
                >
                  <SelectTrigger className="h-9 text-body-sm">
                    <SelectValue>
                      {isBound ? (
                        <span className="flex items-center gap-1.5">
                          <CloudCog className="size-3.5 shrink-0 text-primary" strokeWidth={1.5} />
                          <span className="truncate">{resolved?.name ?? "Managed store"}</span>
                        </span>
                      ) : (
                        <span className="flex items-center gap-1.5">
                          <Package className="size-3.5 shrink-0 text-muted-foreground" strokeWidth={1.5} />
                          <span>Built-in</span>
                        </span>
                      )}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__builtin__">
                      <span className="flex items-center gap-2">
                        <Package className="size-3.5 shrink-0 text-muted-foreground" strokeWidth={1.5} />
                        Built-in
                      </span>
                    </SelectItem>
                    {compatibleStores.map((store) => (
                      <SelectItem key={store.arn} value={store.arn}>
                        <span className="flex items-center gap-2">
                          <CloudCog className="size-3.5 shrink-0 text-muted-foreground" strokeWidth={1.5} />
                          <span>{store.name}</span>
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {/* Resolved status dot — always reserve space to avoid layout shift */}
              <span
                className={cn(
                  "size-2 shrink-0 rounded-full transition-colors",
                  !resolved
                    ? "bg-transparent"
                    : resolved.status === "ready"
                      ? "bg-teal-500"
                      : resolved.status === "error"
                        ? "bg-coral-600"
                        : "bg-yellow-500",
                )}
              />
            </div>
          </div>
        );
      })}

      {/* Hint when no compatible stores exist */}
      {entryNames.length > 0 && stores.length === 0 && (
        <p className="text-[11px] text-muted-foreground px-1 pt-1">
          No managed knowledge stores available. Create one from the Knowledge page to bind it here.
        </p>
      )}
    </div>
  );
}
