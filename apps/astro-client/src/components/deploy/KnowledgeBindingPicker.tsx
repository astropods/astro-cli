import { Link } from "react-router";
import { Database, Package } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Button } from "@/components/ui/button";
import { ErrorPanel } from "@/components/ui/status-panel";
import { ProviderIcon } from "@/components/knowledge/ProviderIcon";
import { PROVIDER_LABELS } from "@/components/knowledge/knowledge-utils";
import type { KnowledgeStore, KnowledgeBindingInfo, KnowledgeProvider } from "@/lib/api";
import { newKnowledgePath } from "@/lib/routes";
import type { KnowledgeBindingMode, KnowledgeBindingModes } from "./changeTracking";

export interface KnowledgeBindingPickerProps {
  entries: Record<string, { provider?: string; binding?: string }>;
  bindings: Record<string, string>;
  modes: KnowledgeBindingModes;
  resolvedBindings: Record<string, KnowledgeBindingInfo>;
  errorKeys?: string[];
  onChange: (bindings: Record<string, string>) => void;
  onModeChange: (entryName: string, mode: KnowledgeBindingMode) => void;
  stores: KnowledgeStore[];
}

export function KnowledgeBindingPicker({
  entries,
  bindings,
  modes,
  resolvedBindings,
  errorKeys,
  onChange,
  onModeChange,
  stores,
}: KnowledgeBindingPickerProps) {
  const entryNames = Object.keys(entries).sort();
  if (entryNames.length === 0) return null;
  const errorKeySet = new Set(errorKeys ?? []);

  return (
    <div className="rounded-[6px] border border-border divide-y divide-border">
      {entryNames.map((name) => (
        <KnowledgeBindingEntry
          key={name}
          name={name}
          entry={entries[name]}
          binding={bindings[name]}
          mode={modes[name]}
          resolvedBinding={resolvedBindings[name]}
          error={errorKeySet.has(name)}
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
          onModeChange={(mode) => onModeChange(name, mode)}
        />
      ))}
    </div>
  );
}

function KnowledgeBindingEntry({
  name,
  entry,
  binding,
  mode: selectedMode,
  resolvedBinding,
  error,
  stores,
  onBind,
  onModeChange,
}: {
  name: string;
  entry: { provider?: string; binding?: string };
  binding: string | undefined;
  mode: KnowledgeBindingMode | undefined;
  resolvedBinding: KnowledgeBindingInfo | undefined;
  error: boolean;
  stores: KnowledgeStore[];
  onBind: (arn: string | null) => void;
  onModeChange: (mode: KnowledgeBindingMode) => void;
}) {
  const provider = (entry.provider ?? resolvedBinding?.provider) as KnowledgeProvider | undefined;
  const providerLabel = provider ? PROVIDER_LABELS[provider] ?? provider : undefined;
  const compatibleStores = stores.filter(
    (s) => s.provider === provider && s.status === "ready"
  );
  const rawArn = binding || (selectedMode ? "" : entry.binding) || "";
  const mode: KnowledgeBindingMode = selectedMode ?? (rawArn ? "shared" : "local");

  return (
    <div className="px-5 py-4">
      <div className="flex items-center gap-3">
        <div className="flex size-10 items-center justify-center rounded-md bg-muted shrink-0">
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
            onModeChange(value as KnowledgeBindingMode);
          }}
          className="shrink-0 [&_button]:cursor-pointer"
        >
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <ToggleGroupItem value="shared">Shared</ToggleGroupItem>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs">
                Uses a knowledge store already connected to this account—the same database can back multiple deployments.
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <ToggleGroupItem value="local">Local</ToggleGroupItem>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs">
                Runs database workloads alongside this deployment (provisioned with the agent). Does not use an account-registered knowledge store.
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </ToggleGroup>
      </div>

      {mode === "shared" && (
        <div className="mt-4 pl-[52px]">
          {compatibleStores.length === 0 && !rawArn ? (
            <div className="flex items-center gap-3 rounded-[6px] border border-dashed border-border bg-muted/30 px-3.5 py-3">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-card">
                <Database className="size-4.5 text-muted-foreground" strokeWidth={1.5} />
              </div>
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <p className="text-[13px] font-medium text-foreground">
                  No {providerLabel ?? "compatible"} knowledge stores connected
                </p>
                <p className="text-[12px] text-muted-foreground">
                  Connect one to share it across deployments, or stay on Local.
                </p>
              </div>
              <Button variant="outline" size="sm" className="shrink-0" asChild>
                <Link to={newKnowledgePath}>Add store</Link>
              </Button>
            </div>
          ) : (
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
                {rawArn && !compatibleStores.some((s) => s.arn === rawArn) && (
                  <SelectItem value={rawArn}>
                    {resolvedBinding?.name ?? rawArn}
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
          )}
          {error && (
            <div className="mt-3">
              <ErrorPanel variant="inline">
                Select a shared knowledge store or switch to Local.
              </ErrorPanel>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
