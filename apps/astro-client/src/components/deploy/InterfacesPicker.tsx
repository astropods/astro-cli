import type { ReactNode } from "react";
import { Check } from "lucide-react";
import { Slack } from "@/components/ui/svgs/slack";
import { AstroMark } from "@/components/ui/svgs/astroMark";
import { cn } from "@/lib/utils";
import { AVAILABLE_ADAPTERS } from "./useDeployForm";
import { VariableFields } from "./VariableFields";
import type { VariableDisplay } from "./VariableFields";
import { GrantsEditor } from "./GrantsEditor";
import type { AccountVariable, AuthGrant } from "@/lib/api";

/** Brand icons manage their own color; generic icons inherit from parent. */
const ADAPTER_ICONS: Record<string, { icon: ReactNode; isBrand?: boolean }> = {
  web: { icon: <AstroMark mono className="h-5 w-5" /> },
  slack: { icon: <Slack className="h-5 w-5" />, isBrand: true },
};

const ADAPTERS_ERROR_ID = "messaging-interface-error";

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
  /** Auth grants per adapter — when present, a grants editor renders inside the adapter card. */
  webGrants?: AuthGrant[];
  onWebGrantsChange?: (grants: AuthGrant[]) => void;
  slackGrants?: AuthGrant[];
  onSlackGrantsChange?: (grants: AuthGrant[]) => void;
  /** Target account name — used by GrantsEditor to scope the user picker to that account's members. */
  targetAccount?: string;
  vaultEntries?: AccountVariable[];
  vaultEntriesLoaded?: boolean;
  vaultSettingsUrl?: string;
  vaultLoadError?: string | null;
  /** Form-level bulk mapper used when the vault picker creates multiple variables at once. */
  bulkSetVariables?: (imported: Record<string, string>) => void;
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
  webGrants,
  onWebGrantsChange,
  slackGrants,
  onSlackGrantsChange,
  targetAccount,
  vaultEntries,
  vaultEntriesLoaded,
  vaultSettingsUrl,
  vaultLoadError,
  bulkSetVariables,
}: InterfacesPickerProps) {
  const toggle = (id: string) => {
    onChange(selected.includes(id) ? selected.filter((a) => a !== id) : [...selected, id]);
  };

  return (
    <div>
      <div
        role="group"
        aria-label="Messaging interface options"
        aria-invalid={showError || undefined}
        aria-describedby={showError ? ADAPTERS_ERROR_ID : undefined}
        className={cn(
          "space-y-2 rounded-[8px] transition-[outline-color]",
          showError && "outline outline-1 outline-destructive",
        )}
      >
      {AVAILABLE_ADAPTERS.map((adapter) => {
        const isSelected = selected.includes(adapter.id);
        const { icon, isBrand = false } = ADAPTER_ICONS[adapter.id] ?? {};
        const credentialEntries = adapterCredDefs[adapter.id] ?? [];
        const hasCredentials = isSelected && credentialEntries.length > 0;
        const credentialLayout = credentialLayoutByAdapter?.[adapter.id] ?? "below";
        const hasInlineCredentials = hasCredentials && credentialLayout === "inline-card";
        const hasBelowCredentials = hasCredentials && credentialLayout === "below";
        const hasGrantsEditor = isSelected && (
          (adapter.id === "web" && onWebGrantsChange !== undefined) ||
          (adapter.id === "slack" && onSlackGrantsChange !== undefined)
        );
        const grantsForAdapter = adapter.id === "web" ? webGrants : slackGrants;
        const onGrantsChangeForAdapter = adapter.id === "web" ? onWebGrantsChange : onSlackGrantsChange;
        const hasInlineSection = hasInlineCredentials || hasGrantsEditor;

        return (
          <div key={adapter.id}>
            <div
              className={cn(
                hasInlineSection && "rounded-[6px] border transition-[border-color,background-color]",
                hasInlineSection &&
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
                  hasInlineSection && "border-none bg-transparent hover:bg-transparent",
                  !hasInlineSection &&
                    (isSelected
                      ? "border-primary/40 bg-primary/5"
                      : "border-border bg-transparent hover:bg-muted/50"),
                )}
              >
                <div className={cn(
                  "flex h-9 w-9 items-center justify-center rounded-sm shrink-0 transition-colors",
                  isSelected ? "bg-primary/10 dark:bg-primary/25" : "bg-muted",
                  !isBrand && (isSelected ? "text-foreground-accent" : "text-muted-foreground"),
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
              {(() => {
                // Slack puts access control above the bot/app token credentials —
                // who can use the bot is the more important first decision.
                // Web shows credentials first (there usually aren't any) then grants.
                const grantsFirst = adapter.id === "slack";
                const credsBlock = hasInlineCredentials ? (
                  <div
                    key="creds"
                    className={cn(
                      "border-t border-primary/20 bg-surface px-6 py-3",
                      ((grantsFirst && !hasInlineCredentials) ||
                        (!grantsFirst && !hasGrantsEditor)) &&
                        "rounded-b-[6px]",
                      // Last block in the stack rounds its bottom corners.
                      grantsFirst && "rounded-b-[6px]",
                    )}
                  >
                    <VariableFields
                      variables={credentialEntries}
                      values={adapterCredentials}
                      onChange={onAdapterCredentialsChange}
                      errorKeys={adapterErrorKeys}
                      account={targetAccount}
                      vaultEntries={vaultEntries}
                      vaultEntriesLoaded={vaultEntriesLoaded}
                      vaultSettingsUrl={vaultSettingsUrl}
                      vaultLoadError={vaultLoadError}
                      bulkSetVariables={bulkSetVariables}
                    />
                  </div>
                ) : null;
                const grantsBlock = hasGrantsEditor ? (
                  <div
                    key="grants"
                    className={cn(
                      "border-t border-primary/20 bg-surface px-6 py-3",
                      // Last block in the stack rounds its bottom corners.
                      !grantsFirst && "rounded-b-[6px]",
                      grantsFirst && !hasInlineCredentials && "rounded-b-[6px]",
                    )}
                  >
                    <GrantsEditor
                      adapter={adapter.id as "web" | "slack"}
                      grants={grantsForAdapter ?? []}
                      onChange={onGrantsChangeForAdapter!}
                      targetAccount={targetAccount}
                    />
                  </div>
                ) : null;
                return grantsFirst ? [grantsBlock, credsBlock] : [credsBlock, grantsBlock];
              })()}
            </div>
            {hasBelowCredentials && (
              <div className="pt-4 pb-4 pl-6">
                <VariableFields
                  variables={credentialEntries}
                  values={adapterCredentials}
                  onChange={onAdapterCredentialsChange}
                  errorKeys={adapterErrorKeys}
                  account={targetAccount}
                  vaultEntries={vaultEntries}
                  vaultEntriesLoaded={vaultEntriesLoaded}
                  vaultSettingsUrl={vaultSettingsUrl}
                  vaultLoadError={vaultLoadError}
                  bulkSetVariables={bulkSetVariables}
                />
              </div>
            )}
          </div>
        );
      })}
      </div>
      {showError && (
        <p id={ADAPTERS_ERROR_ID} className="mt-2 text-xs text-destructive">
          Select at least one messaging type
        </p>
      )}
    </div>
  );
}
