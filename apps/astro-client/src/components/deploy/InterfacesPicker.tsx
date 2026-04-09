import type { ReactNode } from "react";
import { AlertCircle, Check, Globe } from "lucide-react";
import { Slack } from "@/components/ui/svgs/slack";
import { cn } from "@/lib/utils";
import { AVAILABLE_ADAPTERS } from "./useDeployForm";
import { VariableFields } from "./VariableFields";
import type { VariableDisplay } from "./VariableFields";

/** Brand icons manage their own color; generic icons inherit from parent. */
const ADAPTER_ICONS: Record<string, { icon: ReactNode; isBrand?: boolean }> = {
  web: { icon: <Globe className="h-5 w-5" strokeWidth={1.5} /> },
  slack: { icon: <Slack className="h-5 w-5" />, isBrand: true },
};

export interface InterfacesPickerProps {
  selected: string[];
  onChange: (adapters: string[]) => void;
  /** Per-adapter credential field definitions, keyed by adapter id. */
  adapterCredDefs: Record<string, [string, VariableDisplay][]>;
  adapterCredentials: Record<string, string>;
  onAdapterCredentialsChange: (values: Record<string, string>) => void;
  showError?: boolean;
  adapterErrorKeys?: string[];
  /** Controls where adapter credential fields render for each adapter id. */
  credentialLayoutByAdapter?: Record<string, "below" | "inline-card">;
}

export function InterfacesPicker({
  selected,
  onChange,
  adapterCredDefs,
  adapterCredentials,
  onAdapterCredentialsChange,
  showError,
  adapterErrorKeys,
  credentialLayoutByAdapter,
}: InterfacesPickerProps) {
  const toggle = (id: string) => {
    onChange(selected.includes(id) ? selected.filter((a) => a !== id) : [...selected, id]);
  };

  return (
    <div>
      <div className="space-y-2">
      {AVAILABLE_ADAPTERS.map((adapter) => {
        const isSelected = selected.includes(adapter.id);
        const { icon, isBrand = false } = ADAPTER_ICONS[adapter.id] ?? {};
        const credentialEntries = adapterCredDefs[adapter.id] ?? [];
        const hasCredentials = isSelected && credentialEntries.length > 0;
        const credentialLayout = credentialLayoutByAdapter?.[adapter.id] ?? "below";
        const hasInlineCredentials = hasCredentials && credentialLayout === "inline-card";
        const hasBelowCredentials = hasCredentials && credentialLayout === "below";

        return (
          <div key={adapter.id}>
            <div
              className={cn(
                hasInlineCredentials &&
                  "rounded-[6px] border transition-[border-color,background-color]",
                hasInlineCredentials &&
                  (isSelected
                    ? "border-primary/40 bg-primary/5"
                    : "border-border bg-transparent"),
              )}
            >
              <button
                type="button"
                aria-pressed={isSelected}
                onClick={() => toggle(adapter.id)}
                className={cn(
                  "w-full flex items-center gap-4 py-3 px-3 rounded-[6px] border text-left cursor-pointer transition-[border-color,background-color]",
                  hasInlineCredentials && "border-none bg-transparent hover:bg-transparent",
                  !hasInlineCredentials &&
                    (isSelected
                      ? "border-primary/40 bg-primary/5"
                      : "border-border bg-transparent hover:bg-stone-200/50"),
                )}
              >
                <div className={cn(
                  "flex h-9 w-9 items-center justify-center rounded-sm shrink-0 transition-colors",
                  isSelected ? "bg-primary/10" : "bg-stone-200",
                  !isBrand && (isSelected ? "text-primary" : "text-muted-foreground"),
                )}>
                  {icon}
                </div>
                <div className="flex flex-col gap-0.5 flex-1 min-w-0">
                  <span className="text-[13px] font-medium text-foreground">{adapter.label}</span>
                  <span className="text-[11px] text-muted-foreground">{adapter.description}</span>
                </div>
                <div className={cn(
                  "w-5 h-5 rounded border-2 flex items-center justify-center shrink-0 transition-colors",
                  isSelected
                    ? "border-primary bg-primary"
                    : "border-input bg-background",
                )}>
                  {isSelected && <Check size={14} strokeWidth={3} className="text-primary-foreground" />}
                </div>
              </button>
              {hasInlineCredentials && (
                <div className="border-t border-primary/20 rounded-b-[6px] bg-surface px-6 py-3">
                  <VariableFields
                    variables={credentialEntries}
                    values={adapterCredentials}
                    onChange={onAdapterCredentialsChange}
                    errorKeys={adapterErrorKeys}
                  />
                </div>
              )}
            </div>
            {hasBelowCredentials && (
              <div className="pt-4 pb-4 pl-6">
                <VariableFields
                  variables={credentialEntries}
                  values={adapterCredentials}
                  onChange={onAdapterCredentialsChange}
                  errorKeys={adapterErrorKeys}
                />
              </div>
            )}
          </div>
        );
      })}
      </div>
      {showError && (
        <div className="flex items-center gap-1.5 mt-3 px-3 py-2 rounded-[6px] bg-red-50" role="alert">
          <AlertCircle size={14} className="text-red-700 shrink-0" />
          <p className="text-sm text-red-700">
            Select at least one messaging type
          </p>
        </div>
      )}
    </div>
  );
}
