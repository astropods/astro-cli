import { useState, useEffect, useMemo, useRef } from "react";
import { Eye, EyeOff } from "lucide-react";
import { ShieldCheckIcon } from "@heroicons/react/24/outline";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { VaultPicker, VaultRefChip, parseVaultToken } from "./VaultPicker";
import type { VariableDisplay } from "./VariableFields";

/** Convert "SLACK_BOT_TOKEN" → "your-slack-bot-token" */
function placeholderFromKey(key: string): string {
  return "your-" + key.replace(/_/g, "-").toLowerCase();
}

const LABEL_ACRONYMS: Record<string, string> = {
  Api: "API",
  Ids: "IDs",
  Id: "ID",
  Url: "URL",
  Oauth: "OAuth",
  Openai: "OpenAI",
  Ai: "AI",
  Llm: "LLM",
  Db: "DB",
  Sdk: "SDK",
  Jwt: "JWT",
  Github: "GitHub",
};

/** Convert "SLACK_API_KEY" → "Slack API Key", "ORG_IDS" → "Org IDs" */
export function humanizeKey(key: string): string {
  return key
    .replace(/_/g, " ")
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .replace(/\b(Api|Ids|Id|Url|Oauth|Openai|Ai|Llm|Db|Sdk|Jwt|Github)\b/g, (m) => LABEL_ACRONYMS[m] ?? m);
}

/** Returns the implicit default value for a variable based on its type. */
export function getVariableDefault(meta: VariableDisplay): string {
  switch (meta.datatype) {
    case "boolean":
      return "false";
    default:
      return "";
  }
}

function renderFieldIcon(icon?: string) {
  switch (icon) {
    case "shield":
      return <ShieldCheckIcon className="h-4 w-4 text-muted-foreground" />;
    default:
      return null;
  }
}

/** Returns true if a variable is considered filled for validation purposes. */
export function isVariableFilled(meta: VariableDisplay, value: string | undefined): boolean {
  switch (meta.datatype) {
    case "boolean":
      return true;
    default:
      return !!value?.trim();
  }
}

function findVaultSuggestions(fieldKey: string, entries: import("@/lib/api").AccountVariable[]) {
  const key = fieldKey.toLowerCase();
  const exactCase = entries.filter((e) => e.name === fieldKey);
  const exactCI = entries.filter((e) => e.name !== fieldKey && e.name.toLowerCase() === key);
  const best = exactCase;
  const possible = exactCI;
  return { best, possible, all: [...best, ...possible] };
}

function makeVaultToken(entry: import("@/lib/api").AccountVariable): string {
  return entry.secret ? `{{secrets.${entry.name}}}` : `{{vars.${entry.name}}}`;
}

/** Auto-fills the field with the best vault match once on first render when empty. */
function useVaultAutoFill(
  fieldKey: string,
  value: string,
  entries: import("@/lib/api").AccountVariable[],
  onChange: (val: string) => void,
) {
  const suggestions = useMemo(() => findVaultSuggestions(fieldKey, entries), [fieldKey, entries]);
  const didAutoFill = useRef(false);
  const [autoFilledToken, setAutoFilledToken] = useState<string | null>(null);

  useEffect(() => {
    if (!didAutoFill.current && value === "" && suggestions.all.length > 0) {
      const token = makeVaultToken(suggestions.all[0]);
      didAutoFill.current = true;
      setAutoFilledToken(token);
      onChange(token);
    }
  }, [suggestions, value, onChange]);

  // Derived: true only while the value still matches what was auto-filled.
  // Clears automatically when the user picks a different entry or clears the field.
  const isAutoFilled = !!autoFilledToken && value === autoFilledToken;

  return { isAutoFilled, suggestions };
}

/** Builds the inline "auto filled" / "auto filled · N others" label. */
function autoFillLabel(suggestions: import("@/lib/api").AccountVariable[]): string {
  const others = suggestions.length - 1;
  if (others <= 0) return "Auto-filled";
  return `Auto-filled · ${others} other ${others > 1 ? "matches" : "match"}`;
}

export interface VariableFieldProps {
  fieldKey: string;
  meta: VariableDisplay;
  value: string;
  onChange: (value: string) => void;
  hasError?: boolean;
  refInvalid?: boolean;
  account?: string;
  vaultEntries?: import("@/lib/api").AccountVariable[];
  vaultSettingsUrl?: string;
}

export function VariableField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultSettingsUrl }: VariableFieldProps) {
  // 1. Select dropdown
  if (meta.displayAs === "select" && meta.options && meta.options.length > 0) {
    return (
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger
          id={fieldKey}
          aria-invalid={hasError || undefined}
        >
          <SelectValue placeholder={meta.placeholder || "Select an option"} />
        </SelectTrigger>
        <SelectContent>
          {meta.options.map((option) => (
            <SelectItem key={option} value={option}>
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  // 2. Long text / textarea
  if (meta.displayAs === "long-text") {
    return (
      <Textarea
        id={fieldKey}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
        autoComplete="off"
        spellCheck={false}
        aria-invalid={hasError || undefined}
      />
    );
  }

  // 3. Boolean toggle
  if (meta.datatype === "boolean") {
    return (
      <div className="flex items-center justify-between">
        <label htmlFor={fieldKey} className="cursor-pointer">
          <div className="flex items-center gap-2">
            {meta.icon ? <span className="shrink-0">{renderFieldIcon(meta.icon)}</span> : null}
            <div>
              <p className="text-[13px] font-medium text-foreground select-none">
                {meta.label ?? humanizeKey(fieldKey)}
              </p>
              {meta.description ? (
                <p className="mt-0.5 text-[11px] text-muted-foreground">
                  {meta.description}
                </p>
              ) : null}
            </div>
          </div>
        </label>
        <Switch
          id={fieldKey}
          checked={value === "true"}
          onCheckedChange={(checked) => onChange(checked ? "true" : "false")}
        />
      </div>
    );
  }

  // 4. Number input
  if (meta.datatype === "number") {
    return (
      <Input
        id={fieldKey}
        type="number"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
        autoComplete="off"
        aria-invalid={hasError || undefined}
      />
    );
  }

  // 5. Array / object — textarea with hint placeholder
  if (meta.datatype === "array" || meta.datatype === "object") {
    const placeholder =
      meta.placeholder ||
      (meta.datatype === "array" ? '["item1", "item2"]' : '{"key": "value"}');
    return (
      <Textarea
        id={fieldKey}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoComplete="off"
        spellCheck={false}
        aria-invalid={hasError || undefined}
      />
    );
  }

  // 6. Secret (password) input with reveal toggle
  if (meta.secret) {
    return <SecretField fieldKey={fieldKey} meta={meta} value={value} onChange={onChange} hasError={hasError} refInvalid={refInvalid} account={account} vaultEntries={vaultEntries} vaultSettingsUrl={vaultSettingsUrl} />;
  }

  // 7. Default — text input
  return <DefaultTextField fieldKey={fieldKey} meta={meta} value={value} onChange={onChange} hasError={hasError} refInvalid={refInvalid} account={account} vaultEntries={vaultEntries} vaultSettingsUrl={vaultSettingsUrl} />;
}

function DefaultTextField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultSettingsUrl }: VariableFieldProps) {
  const { isAutoFilled, suggestions } = useVaultAutoFill(fieldKey, value, vaultEntries ?? [], onChange);
  const [pickerOpen, setPickerOpen] = useState(false);
  const isVaultRef = parseVaultToken(value) !== null;
  const bestMatchNames = suggestions.best.map((s) => s.name);
  const possibleMatchNames = suggestions.possible.map((s) => s.name);
  const selectedName = parseVaultToken(value)?.name;

  return (
    <div className="relative flex items-center">
      {isVaultRef ? (
        <VaultRefChip
          token={value}
          onClear={() => onChange("")}
          invalid={hasError || refInvalid}
          autoFillLabel={isAutoFilled ? autoFillLabel(suggestions.all) : undefined}
          onAutoFillClick={isAutoFilled && suggestions.all.length > 1 ? () => setPickerOpen(true) : undefined}
        />
      ) : (
        <Input
          id={fieldKey}
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value.trim())}
          placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={hasError || undefined}
          className="pr-9"
        />
      )}
      <div className="absolute right-2">
        <VaultPicker onSelect={onChange} entries={vaultEntries} accountName={account} vaultSettingsUrl={vaultSettingsUrl} bestMatchNames={bestMatchNames.length > 0 ? bestMatchNames : undefined} possibleMatchNames={possibleMatchNames.length > 0 ? possibleMatchNames : undefined} selectedName={selectedName} open={pickerOpen} onOpenChange={setPickerOpen} />
      </div>
    </div>
  );
}

function SecretField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultSettingsUrl }: VariableFieldProps) {
  const [revealed, setRevealed] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const { isAutoFilled, suggestions } = useVaultAutoFill(fieldKey, value, vaultEntries ?? [], onChange);
  const isVaultRef = parseVaultToken(value) !== null;
  const bestMatchNames = suggestions.best.map((s) => s.name);
  const possibleMatchNames = suggestions.possible.map((s) => s.name);
  const selectedName = parseVaultToken(value)?.name;

  return (
    <div className="relative flex items-center">
      {isVaultRef ? (
        <VaultRefChip
          token={value}
          onClear={() => onChange("")}
          invalid={hasError || refInvalid}
          autoFillLabel={isAutoFilled ? autoFillLabel(suggestions.all) : undefined}
          onAutoFillClick={isAutoFilled && suggestions.all.length > 1 ? () => setPickerOpen(true) : undefined}
        />
      ) : (
        <Input
          id={fieldKey}
          type={revealed ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value.trim())}
          placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
          className="pr-16"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={hasError || undefined}
        />
      )}
      <div className="absolute right-2 flex items-center gap-3">
        {!isVaultRef && (
          <button
            type="button"
            onClick={() => setRevealed((r) => !r)}
            className="text-muted-foreground hover:text-foreground transition-colors"
            aria-label={revealed ? "Hide value" : "Reveal value"}
          >
            {revealed ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
          </button>
        )}
        <VaultPicker onSelect={onChange} entries={vaultEntries} accountName={account} vaultSettingsUrl={vaultSettingsUrl} bestMatchNames={bestMatchNames.length > 0 ? bestMatchNames : undefined} possibleMatchNames={possibleMatchNames.length > 0 ? possibleMatchNames : undefined} selectedName={selectedName} open={pickerOpen} onOpenChange={setPickerOpen} />
      </div>
    </div>
  );
}
