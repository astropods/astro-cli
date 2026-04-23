import { useState } from "react";
import { Package } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
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
    <div className="rounded-[6px] border border-border divide-y divide-border">
      {entryNames.map((name) => (
        <KnowledgeBindingEntry
          key={name}
          name={name}
          entry={entries[name]}
          binding={bindings[name]}
          resolvedBinding={resolvedBindings[name]}
          stores={stores}
          onBind={(arn) => {
            const next = { ...bindings };
            if (arn) {
              next[name] = arn;
            } else {
              delete next[name];
            }
            onChange(next);
          }}
        />
      ))}
    </div>
  );
}

function KnowledgeBindingEntry({
  name,
  entry,
  binding,
  resolvedBinding,
  stores,
  onBind,
}: {
  name: string;
  entry: { provider?: string; binding?: string };
  binding: string | undefined;
  resolvedBinding: KnowledgeBindingInfo | undefined;
  stores: KnowledgeStore[];
  onBind: (arn: string | null) => void;
}) {
  const provider = (entry.provider ?? resolvedBinding?.provider) as KnowledgeProvider | undefined;
  const providerLabel = provider ? PROVIDER_LABELS[provider] ?? provider : undefined;
  const compatibleStores = stores.filter(
    (s) => s.provider === provider && s.status === "ready"
  );
  const rawArn = binding || entry.binding || "";
  const isBound = rawArn !== "";
  const [mode, setMode] = useState<"new" | "existing">(isBound ? "existing" : "new");

  return (
    <div className="px-5 py-4">
      <div className="flex items-center gap-3">
        <div className="flex size-10 items-center justify-center rounded-md bg-stone-200 shrink-0">
          {provider ? (
            <ProviderIcon provider={provider} className="size-5" />
          ) : (
            <Package className="size-5 text-muted-foreground" strokeWidth={1.5} />
          )}
        </div>
        <div className="flex flex-col gap-0.5 min-w-0 flex-1">
          <span className="text-[14px] font-semibold text-foreground truncate">{name}</span>
          {providerLabel && (
            <span className="text-[13px] font-normal text-muted-foreground">{providerLabel}</span>
          )}
        </div>
        <ToggleGroup
          type="single"
          variant="word"
          value={mode}
          onValueChange={(value) => {
            if (!value) return;
            const next = value as "new" | "existing";
            setMode(next);
            if (next === "new") {
              onBind(null);
            }
          }}
          className="shrink-0 [&_button]:cursor-pointer"
        >
          <ToggleGroupItem value="new">Built in</ToggleGroupItem>
          <ToggleGroupItem value="existing">Existing</ToggleGroupItem>
        </ToggleGroup>
      </div>

      {mode === "existing" && (
        <div className="mt-4 pl-[52px]">
          <Select
            value={rawArn || undefined}
            onValueChange={(value) => onBind(value)}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Select a store" />
            </SelectTrigger>
            <SelectContent>
              {compatibleStores.map((store) => (
                <SelectItem key={store.arn} value={store.arn}>
                  {store.name}
                </SelectItem>
              ))}
              {compatibleStores.length === 0 && (
                <SelectItem value="__empty__" disabled>
                  No compatible stores
                </SelectItem>
              )}
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  );
}
