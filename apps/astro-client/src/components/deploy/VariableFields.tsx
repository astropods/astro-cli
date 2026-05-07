import { useRef, useCallback } from "react";
import { Info, ExternalLink } from "lucide-react";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "@/components/ui/tooltip";
import { Label } from "@/components/ui/label";
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
}

/** Convert "SLACK_BOT_TOKEN" → "Slack Bot Token" */

export interface VariableFieldsProps {
  variables: [string, VariableDisplay][];
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
  errorKeys?: string[];
  invalidRefKeys?: string[];
  account?: string;
  vaultEntries?: AccountVariable[];
  vaultSettingsUrl?: string;
  vaultLoadError?: string | null;
}

export function VariableFields({ variables, values, onChange, errorKeys, invalidRefKeys, account, vaultEntries, vaultSettingsUrl, vaultLoadError }: VariableFieldsProps) {
  if (variables.length === 0) return null;

  // Keep a ref so per-field onChange callbacks always see the latest values,
  // preventing stale-closure overwrites when multiple fields auto-fill in the
  // same effect batch.
  const valuesRef = useRef(values);
  valuesRef.current = values;

  const handleFieldChange = useCallback((key: string, val: string) => {
    const updated = { ...valuesRef.current, [key]: val };
    valuesRef.current = updated;
    onChange(updated);
  }, [onChange]);

  return (
    <TooltipProvider delayDuration={200}>
      <div className="space-y-5">
        {variables.map(([key, v]) => (
          <div key={key}>
            {v.datatype !== "boolean" && (
              <div className="flex items-center justify-between mb-1">
                <div className="flex items-center gap-1.5">
                  <Label htmlFor={key} size="md" className="mb-0">
                    {v.label ?? humanizeKey(key)}
                  </Label>
                  {v.optional && (
                    <span className="text-xs text-muted-foreground">Optional</span>
                  )}
                  {v.description && (
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
                    className="text-xs text-teal-700 hover:text-teal-900 flex items-center gap-1"
                  >
                    Where do I find this?
                    <ExternalLink className="h-3 w-3" />
                  </a>
                )}
              </div>
            )}
            <VariableField
              fieldKey={key}
              meta={v}
              value={values[key] || ""}
              onChange={(val) => handleFieldChange(key, val)}
              hasError={errorKeys?.includes(key)}
              refInvalid={invalidRefKeys?.includes(key)}
              account={account}
              vaultEntries={vaultEntries}
              vaultSettingsUrl={vaultSettingsUrl}
              vaultLoadError={vaultLoadError}
            />
            {errorKeys?.includes(key) && (
              <p className="text-destructive text-xs mt-1">
                {values[key] ? 'Variable not found in selected account' : 'Required'}
              </p>
            )}
          </div>
        ))}
      </div>
    </TooltipProvider>
  );
}
