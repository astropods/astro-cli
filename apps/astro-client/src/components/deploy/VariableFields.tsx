import { Info, ExternalLink } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider,
} from "@/components/ui/tooltip";
/** Display-only variable metadata — only the fields the component actually renders. */
export interface VariableDisplay {
  description?: string;
  optional?: boolean;
  label?: string;
  placeholder?: string;
  helpUrl?: string;
}

/** Convert "SLACK_BOT_TOKEN" → "Slack Bot Token" */
function humanizeKey(key: string): string {
  return key
    .replace(/_/g, " ")
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

/** Convert "SLACK_BOT_TOKEN" → "your-slack-bot-token" */
function placeholderFromKey(key: string): string {
  return "your-" + key.replace(/_/g, "-").toLowerCase();
}

export interface VariableFieldsProps {
  variables: [string, VariableDisplay][];
  values: Record<string, string>;
  onChange: (values: Record<string, string>) => void;
  errorKeys?: string[];
}

export function VariableFields({ variables, values, onChange, errorKeys }: VariableFieldsProps) {
  if (variables.length === 0) return null;

  return (
    <TooltipProvider delayDuration={200}>
      <div className="rounded-[6px] bg-stone-100 p-4">
        {variables.map(([key, v]) => (
          <div key={key} className="pt-5 first:pt-0">
            <div className="flex items-center justify-between mb-1.5">
              <div className="flex items-center gap-1.5">
                <label htmlFor={key} className="text-sm font-medium text-foreground">
                  {v.label ?? humanizeKey(key)}
                </label>
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
              <div className="flex items-center gap-3">
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
                {v.optional && (
                  <span className="text-xs text-muted-foreground">Optional</span>
                )}
              </div>
            </div>
            <Input
              id={key}
              type="password"
              autoComplete="off"
              spellCheck={false}
              value={values[key] || ""}
              onChange={(e) => onChange({ ...values, [key]: e.target.value })}
              placeholder={v.placeholder || placeholderFromKey(key)}
              className="bg-white font-mono"
              aria-invalid={errorKeys?.includes(key) || undefined}
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
