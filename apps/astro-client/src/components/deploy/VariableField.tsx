import { useState, useEffect, useMemo, useCallback } from "react";
import { Eye, EyeOff } from "lucide-react";
import { ShieldCheckIcon } from "@heroicons/react/24/outline";
import type { AccountVariable } from "@/lib/api";
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
import { ConfiguredInlineSecretChip, VaultPicker, VaultRefChip, parseVaultToken } from "./VaultPicker";
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
  Grc: "GRC",
};

/** Convert "SLACK_API_KEY" → "Slack API Key", "ORG_IDS" → "Org IDs" */
export function humanizeKey(key: string): string {
  return key
    .replace(/_/g, " ")
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .replace(/\b(Api|Ids|Id|Url|Oauth|Openai|Ai|Llm|Db|Sdk|Jwt|Github|Grc)\b/g, (m) => LABEL_ACRONYMS[m] ?? m);
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
      if (meta.secret && meta.configured && !value?.trim()) {
        return true;
      }
      return !!value?.trim();
  }
}

function findVaultSuggestions(
  fieldKey: string,
  entries: AccountVariable[],
  expectedSecret: boolean,
) {
  const key = fieldKey.toLowerCase();
  const compatibleEntries = entries.filter((entry) => entry.secret === expectedSecret);
  const exactCase = compatibleEntries.filter((e) => e.name === fieldKey);
  const exactCI = compatibleEntries.filter((e) => e.name !== fieldKey && e.name.toLowerCase() === key);
  const best = exactCase;
  const possible = exactCI;
  return { best, possible, all: [...best, ...possible] };
}

function makeVaultToken(entry: AccountVariable): string {
  return entry.secret ? `{{secrets.${entry.name}}}` : `{{vars.${entry.name}}}`;
}

/** Offers one auto-fill opportunity after a fresh form has finished seeding.
 *
 * Once entries are loaded, the opportunity is consumed whether or not a fill
 * occurs. That prevents clearing a prefilled or user-entered value from
 * resurrecting an auto-suggestion later. Explicit user changes also clear the
 * provenance marker, including selecting the same entry from the picker.
 */
function useVaultAutoFill(
  fieldKey: string,
  value: string,
  expectedSecret: boolean,
  entries: AccountVariable[],
  entriesLoaded: boolean,
  enabled: boolean,
  onChange: (val: string) => void,
  controlledAutoFilledToken?: string | null,
  onAutoFilledTokenChange?: (token: string | null) => void,
) {
  const suggestions = useMemo(
    () => findVaultSuggestions(fieldKey, entries, expectedSecret),
    [fieldKey, entries, expectedSecret],
  );
  const [localAutoFilledToken, setLocalAutoFilledToken] = useState<string | null>();
  const autoFilledToken = onAutoFilledTokenChange
    ? controlledAutoFilledToken
    : localAutoFilledToken;
  const setAutoFilledToken = useCallback(
    (token: string | null) => {
      if (onAutoFilledTokenChange) {
        onAutoFilledTokenChange(token);
      } else {
        setLocalAutoFilledToken(token);
      }
    },
    [onAutoFilledTokenChange],
  );

  useEffect(() => {
    if (!enabled || !entriesLoaded || autoFilledToken !== undefined) return;
    if (value !== "" || suggestions.all.length === 0) {
      setAutoFilledToken(null);
      return;
    }

    const token = makeVaultToken(suggestions.all[0]);
    setAutoFilledToken(token);
    onChange(token);
  }, [
    autoFilledToken,
    enabled,
    entriesLoaded,
    suggestions,
    value,
    onChange,
    setAutoFilledToken,
  ]);

  const handleUserChange = useCallback((nextValue: string) => {
    setAutoFilledToken(null);
    onChange(nextValue);
  }, [onChange, setAutoFilledToken]);

  const isAutoFilled =
    typeof autoFilledToken === "string" && value === autoFilledToken;

  return { isAutoFilled, suggestions, handleUserChange };
}

/** Builds the inline "auto filled" / "auto filled · N others" label. */
function autoFillLabel(suggestions: AccountVariable[]): string {
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
  vaultEntries?: AccountVariable[];
  vaultEntriesLoaded?: boolean;
  vaultSettingsUrl?: string;
  vaultLoadError?: string | null;
  /** Auto-fill is enabled only for a seeded, fresh deploy form. */
  vaultAutoFillEnabled?: boolean;
  /** Form-owned auto-fill provenance; undefined means this field has not been evaluated. */
  vaultAutoFilledToken?: string | null;
  /** Persists auto-fill provenance across field unmounts. */
  onVaultAutoFilledTokenChange?: (token: string | null) => void;
  /** Form-level bulk mapper used when the vault picker creates multiple variables at once. */
  bulkSetVariables?: (imported: Record<string, string>) => void;
}

export function VariableField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultEntriesLoaded, vaultSettingsUrl, vaultLoadError, vaultAutoFillEnabled = false, vaultAutoFilledToken, onVaultAutoFilledTokenChange, bulkSetVariables }: VariableFieldProps) {
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
        name={fieldKey}
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
        name={fieldKey}
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
        name={fieldKey}
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
    return <SecretField fieldKey={fieldKey} meta={meta} value={value} onChange={onChange} hasError={hasError} refInvalid={refInvalid} account={account} vaultEntries={vaultEntries} vaultEntriesLoaded={vaultEntriesLoaded} vaultSettingsUrl={vaultSettingsUrl} vaultLoadError={vaultLoadError} vaultAutoFillEnabled={vaultAutoFillEnabled} vaultAutoFilledToken={vaultAutoFilledToken} onVaultAutoFilledTokenChange={onVaultAutoFilledTokenChange} bulkSetVariables={bulkSetVariables} />;
  }

  // 7. Default — text input
  return <DefaultTextField fieldKey={fieldKey} meta={meta} value={value} onChange={onChange} hasError={hasError} refInvalid={refInvalid} account={account} vaultEntries={vaultEntries} vaultEntriesLoaded={vaultEntriesLoaded} vaultSettingsUrl={vaultSettingsUrl} vaultLoadError={vaultLoadError} vaultAutoFillEnabled={vaultAutoFillEnabled} vaultAutoFilledToken={vaultAutoFilledToken} onVaultAutoFilledTokenChange={onVaultAutoFilledTokenChange} bulkSetVariables={bulkSetVariables} />;
}

function DefaultTextField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultEntriesLoaded, vaultSettingsUrl, vaultLoadError, vaultAutoFillEnabled, vaultAutoFilledToken, onVaultAutoFilledTokenChange, bulkSetVariables }: VariableFieldProps) {
  const vaultReferenceAllowed = meta.vaultReferenceAllowed !== false;
  const { isAutoFilled, suggestions, handleUserChange } = useVaultAutoFill(
    fieldKey,
    value,
    false,
    vaultEntries ?? [],
    vaultEntriesLoaded ?? false,
    (vaultAutoFillEnabled ?? false) && vaultReferenceAllowed,
    onChange,
    vaultAutoFilledToken,
    onVaultAutoFilledTokenChange,
  );
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
          onClear={() => handleUserChange("")}
          invalid={hasError || refInvalid}
          autoFillLabel={isAutoFilled ? autoFillLabel(suggestions.all) : undefined}
          onAutoFillClick={isAutoFilled && suggestions.all.length > 1 ? () => setPickerOpen(true) : undefined}
        />
      ) : (
        <Input
          id={fieldKey}
          name={fieldKey}
          type="text"
          value={value}
          onChange={(e) => handleUserChange(e.target.value.trim())}
          placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={hasError || undefined}
          className={vaultReferenceAllowed ? "pr-9" : undefined}
        />
      )}
      {vaultReferenceAllowed && (
        <div className="absolute right-2">
          <VaultPicker onSelect={handleUserChange} entries={vaultEntries} expectedSecret={false} accountName={account} vaultSettingsUrl={vaultSettingsUrl} loadError={vaultLoadError} bestMatchNames={bestMatchNames.length > 0 ? bestMatchNames : undefined} possibleMatchNames={possibleMatchNames.length > 0 ? possibleMatchNames : undefined} selectedName={selectedName} open={pickerOpen} onOpenChange={setPickerOpen} bulkSetVariables={bulkSetVariables} newVarName={fieldKey} newVarValue={isVaultRef ? "" : value} newVarSecret={meta.secret ?? false} />
        </div>
      )}
    </div>
  );
}

function SecretField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultEntriesLoaded, vaultSettingsUrl, vaultLoadError, vaultAutoFillEnabled, vaultAutoFilledToken, onVaultAutoFilledTokenChange, bulkSetVariables }: VariableFieldProps) {
  const [revealed, setRevealed] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const vaultReferenceAllowed = meta.vaultReferenceAllowed !== false;
  const { isAutoFilled, suggestions, handleUserChange } = useVaultAutoFill(
    fieldKey,
    value,
    true,
    vaultEntries ?? [],
    vaultEntriesLoaded ?? false,
    (vaultAutoFillEnabled ?? false) && !meta.configured && vaultReferenceAllowed,
    onChange,
    vaultAutoFilledToken,
    onVaultAutoFilledTokenChange,
  );
  const isVaultRef = parseVaultToken(value) !== null;
  const showConfiguredChip =
    meta.configured && !isVaultRef && !value?.trim() && !editing;
  const fieldLabel = meta.label || humanizeKey(fieldKey);

  const bestMatchNames = suggestions.best.map((s) => s.name);
  const possibleMatchNames = suggestions.possible.map((s) => s.name);
  const selectedName = parseVaultToken(value)?.name;

  const exitEditIfStillEmpty = () => {
    if (meta.configured && !value?.trim()) {
      setEditing(false);
      setRevealed(false);
    }
  };

  return (
    <div className="relative flex items-center">
      {isVaultRef ? (
        <VaultRefChip
          token={value}
          onClear={() => handleUserChange("")}
          invalid={hasError || refInvalid}
          autoFillLabel={isAutoFilled ? autoFillLabel(suggestions.all) : undefined}
          onAutoFillClick={isAutoFilled && suggestions.all.length > 1 ? () => setPickerOpen(true) : undefined}
        />
      ) : showConfiguredChip ? (
        <ConfiguredInlineSecretChip
          label={fieldLabel}
          onReplace={() => {
            setEditing(true);
            setRevealed(false);
          }}
          invalid={hasError || refInvalid}
        />
      ) : (
        <Input
          id={fieldKey}
          name={fieldKey}
          type={revealed ? "text" : "password"}
          value={value}
          onBlur={exitEditIfStillEmpty}
          onChange={(e) => handleUserChange(e.target.value.trim())}
          placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
          className={vaultReferenceAllowed ? "pr-16" : "pr-9"}
          autoComplete="new-password"
          spellCheck={false}
          aria-invalid={hasError || undefined}
          autoFocus={editing && meta.configured}
        />
      )}
      <div className="absolute right-2 flex items-center gap-3">
        {!isVaultRef && !showConfiguredChip && value.length > 0 && (
          <button
            type="button"
            onClick={() => setRevealed((r) => !r)}
            className="text-muted-foreground hover:text-foreground transition-colors"
            aria-label={revealed ? "Hide value" : "Reveal value"}
          >
            {revealed ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
          </button>
        )}
        {vaultReferenceAllowed && (
          <VaultPicker onSelect={handleUserChange} entries={vaultEntries} expectedSecret={true} accountName={account} vaultSettingsUrl={vaultSettingsUrl} loadError={vaultLoadError} bestMatchNames={bestMatchNames.length > 0 ? bestMatchNames : undefined} possibleMatchNames={possibleMatchNames.length > 0 ? possibleMatchNames : undefined} selectedName={selectedName} open={pickerOpen} onOpenChange={setPickerOpen} bulkSetVariables={bulkSetVariables} newVarName={fieldKey} newVarValue={isVaultRef ? "" : value} newVarSecret={meta.secret ?? false} />
        )}
      </div>
    </div>
  );
}
