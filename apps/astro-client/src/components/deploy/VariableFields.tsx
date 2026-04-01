import { Info, ExternalLink } from "lucide-react";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "@/components/ui/tooltip";
import { Label } from "@/components/ui/label";
import { VariableField } from "./VariableField";

/** Display-only variable metadata — only the fields the component actually renders. */
export interface VariableDisplay {
  description?: string;
  optional?: boolean;
  secret?: boolean;
  label?: string;
  placeholder?: string;
  helpUrl?: string;
  datatype?: string;
  displayAs?: string;
  options?: string[];
  defaultValue?: string;
}

/** Convert "SLACK_BOT_TOKEN" → "Slack Bot Token" */
function humanizeKey(key: string): string {
  return key
    .replace(/_/g, " ")
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export interface VariableFieldsProps {
  variables: [string, VariableDisplay][];
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
  errorKeys?: string[];
  account?: string;
}

export function VariableFields({ variables, values, onChange, errorKeys, account }: VariableFieldsProps) {
  if (variables.length === 0) return null;

  return (
    <TooltipProvider delayDuration={200}>
      <div className="space-y-5">
        {variables.map(([key, v]) => (
          <div key={key}>
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
            <VariableField
              fieldKey={key}
              meta={v}
              value={values[key] || ""}
              onChange={(val) => onChange({ ...values, [key]: val })}
              hasError={errorKeys?.includes(key)}
              account={account}
            />
            {errorKeys?.includes(key) && (
              <p className="text-destructive text-xs mt-1">Required</p>
            )}
          </div>
        ))}
      </div>
    </TooltipProvider>
  );
}
