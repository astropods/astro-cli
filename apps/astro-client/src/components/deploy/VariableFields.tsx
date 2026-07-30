import { useCallback } from "react";
import type { Dispatch, SetStateAction } from "react";
import { Info, ExternalLink } from "lucide-react";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "@/components/ui/tooltip";
import { Label } from "@/components/ui/label";
import { InlineBadge } from "@/components/InlineBadge";
import { cn } from "@/lib/utils";
import { VariableField, humanizeKey } from "./VariableField";
import type { AccountVariable } from "@/lib/api";

/** Display-only variable metadata — only the fields the component actually renders. */
export interface VariableDisplay {
  description?: string;
  optional?: boolean;
  secret?: boolean;
  label?: string;
  icon?: string;
  placeholder?: string;
  helpUrl?: string;
  datatype?: string;
  displayAs?: string;
  options?: string[];
  defaultValue?: string;
  // Non-empty when the variable/sub-field is deprecated; carries the
  // migration message shown in a tooltip next to the "Deprecated" badge.
  deprecated?: string;
  /** Inline secret is stored; value is not returned by the API. */
  configured?: boolean;
  /** False for display-only fields that cannot be submitted as vault references. */
  vaultReferenceAllowed?: boolean;
}

/** Auto-filled token by field. Missing means not evaluated; null means consumed. */
export type VaultAutoFillState = Record<string, string | null>;

/** Convert "SLACK_BOT_TOKEN" → "Slack Bot Token" */

export interface VariableFieldsProps {
  variables: [string, VariableDisplay][];
  values: Record<string, string>;
  onChange: Dispatch<SetStateAction<Record<string, string>>>;
  errorKeys?: string[];
  invalidRefKeys?: string[];
  account?: string;
  vaultEntries?: AccountVariable[];
  vaultEntriesLoaded?: boolean;
  vaultSettingsUrl?: string;
  vaultLoadError?: string | null;
  vaultAutoFillEnabled?: boolean;
  vaultAutoFillState?: VaultAutoFillState;
  onVaultAutoFillStateChange?: (fieldKey: string, token: string | null) => void;
  /** Form-level bulk mapper used when the vault picker creates multiple variables at once. */
  bulkSetVariables?: (imported: Record<string, string>) => void;
}

export function VariableFields({ variables, values, onChange, errorKeys, invalidRefKeys, account, vaultEntries, vaultEntriesLoaded, vaultSettingsUrl, vaultLoadError, vaultAutoFillEnabled, vaultAutoFillState, onVaultAutoFillStateChange, bulkSetVariables }: VariableFieldsProps) {
  const handleFieldChange = useCallback((key: string, val: string) => {
    // Functional updates compose when several fields auto-fill in one effect
    // turn, including fields rendered by separate required/optional sections.
    onChange((current) => ({ ...current, [key]: val }));
  }, [onChange]);

  if (variables.length === 0) return null;

  return (
    <TooltipProvider delayDuration={200}>
      <div className="space-y-5">
        {variables.map(([key, v]) => (
          <div key={key}>
            {v.datatype !== "boolean" && (
              <div className="flex items-center justify-between mb-1">
                <div className="flex items-center gap-1.5">
                  <Label
                    htmlFor={key}
                    size="md"
                    className={cn("mb-0", v.deprecated && "line-through text-muted-foreground")}
                  >
                    {v.label ?? humanizeKey(key)}
                  </Label>
                  {v.deprecated && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span tabIndex={0} className="inline-flex cursor-help">
                          <InlineBadge variant="soft" className="text-muted-foreground border-border">
                            Deprecated
                          </InlineBadge>
                        </span>
                      </TooltipTrigger>
                      <TooltipContent side="top" sideOffset={4}>
                        {v.deprecated}
                      </TooltipContent>
                    </Tooltip>
                  )}
                  {v.optional && !v.deprecated && (
                    <span className="text-xs text-muted-foreground">Optional</span>
                  )}
                  {v.description && !v.deprecated && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Info className="h-3.5 w-3.5 text-muted-foreground cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent side="top" sideOffset={4}>
                        {v.description}
                      </TooltipContent>
                    </Tooltip>
                  )}
                </div>
                {v.helpUrl && (
                  <a
                    href={v.helpUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-xs text-foreground hover:underline flex items-center gap-1"
                  >
                    Where do I find this?
                    <ExternalLink className="h-3 w-3" />
                  </a>
                )}
              </div>
            )}
            <div className={cn(v.deprecated && "opacity-60 focus-within:opacity-100 transition-opacity")}>
              <VariableField
                fieldKey={key}
                meta={v}
                value={values[key] || ""}
                onChange={(val) => handleFieldChange(key, val)}
                hasError={errorKeys?.includes(key)}
                refInvalid={invalidRefKeys?.includes(key)}
                account={account}
                vaultEntries={vaultEntries}
                vaultEntriesLoaded={vaultEntriesLoaded}
                vaultSettingsUrl={vaultSettingsUrl}
                vaultLoadError={vaultLoadError}
                vaultAutoFillEnabled={vaultAutoFillEnabled}
                vaultAutoFilledToken={vaultAutoFillState?.[key]}
                onVaultAutoFilledTokenChange={
                  onVaultAutoFillStateChange
                    ? (token) => onVaultAutoFillStateChange(key, token)
                    : undefined
                }
                bulkSetVariables={bulkSetVariables}
              />
            </div>
            {errorKeys?.includes(key) && (
              <p className="text-destructive text-xs mt-1">
                {values[key] ? 'Invalid variable reference for this field' : 'Required'}
              </p>
            )}
          </div>
        ))}
      </div>
    </TooltipProvider>
  );
}
