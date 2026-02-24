import type { ReactNode } from "react";
import { AlertCircle, Check, Globe } from "lucide-react";
import { Slack } from "@/components/ui/svgs/slack";
import { AVAILABLE_ADAPTERS, ADAPTER_CREDENTIALS } from "./useDeployForm";
import { VariableFields } from "./VariableFields";
import type { VariableDisplay } from "./VariableFields";

const ADAPTER_ICONS: Record<string, ReactNode> = {
  web: <Globe className="h-5 w-5 text-muted-foreground" strokeWidth={1.5} />,
  slack: <Slack className="h-5 w-5" />,
};

export interface InterfacesPickerProps {
  selected: string[];
  onChange: (adapters: string[]) => void;
  adapterCredentials: Record<string, string>;
  onAdapterCredentialsChange: (values: Record<string, string>) => void;
  showError?: boolean;
  adapterErrorKeys?: string[];
}

export function InterfacesPicker({
  selected,
  onChange,
  adapterCredentials,
  onAdapterCredentialsChange,
  showError,
  adapterErrorKeys,
}: InterfacesPickerProps) {
  const toggle = (id: string) => {
    onChange(selected.includes(id) ? selected.filter((a) => a !== id) : [...selected, id]);
  };

  return (
    <div>
      <div className="[&>div+div]:relative [&>div+div]:before:content-[''] [&>div+div]:before:absolute [&>div+div]:before:top-0 [&>div+div]:before:left-3 [&>div+div]:before:right-3 [&>div+div]:before:border-t [&>div+div]:before:border-border">
      {AVAILABLE_ADAPTERS.map((adapter) => {
        const isSelected = selected.includes(adapter.id);
        const icon = ADAPTER_ICONS[adapter.id];
        const creds = ADAPTER_CREDENTIALS[adapter.id];
        const hasCredentials = isSelected && creds && creds.length > 0;

        const credentialEntries: [string, VariableDisplay][] = creds
          ? creds.map((c) => [c.key, { description: c.description, optional: false, label: c.label, placeholder: c.placeholder, helpUrl: c.helpUrl }])
          : [];

        return (
          <div key={adapter.id}>
            <button
              type="button"
              aria-pressed={isSelected}
              onClick={() => toggle(adapter.id)}
              className="w-[calc(100%+1.5rem)] flex items-center gap-4 py-4 pl-3 pr-6 -ml-3 -mr-3 rounded-[6px] text-left cursor-pointer transition-colors hover:bg-primary/5"
            >
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-stone-100 shrink-0">
                {icon}
              </div>
              <div className="flex flex-col gap-0.5 flex-1 min-w-0">
                <span className="text-sm font-medium text-foreground">{adapter.label}</span>
                <span className="text-xs text-muted-foreground">{adapter.description}</span>
              </div>
              <div className={`w-5 h-5 rounded border-2 flex items-center justify-center shrink-0 transition-colors ${
                isSelected
                  ? "border-primary bg-primary"
                  : "border-input bg-background"
              }`}>
                {isSelected && <Check size={14} strokeWidth={3} className="text-primary-foreground" />}
              </div>
            </button>
            {hasCredentials && (
              <div className="pt-2 pb-4">
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
