import { useState } from "react";
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

/** Convert "WEB_REQUIRE_AUTH" → "Web Require Auth" */
function labelFromKey(key: string): string {
  return key
    .replace(/_/g, " ")
    .toLowerCase()
    .replace(/\b\w/g, (c) => c.toUpperCase());
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
                {meta.label ?? labelFromKey(fieldKey)}
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

  // 7. Default — text input with vault picker
  const isVaultRef = parseVaultToken(value) !== null;
  return (
    <div className="relative flex items-center">
      {isVaultRef ? (
        <VaultRefChip token={value} onClear={() => onChange("")} invalid={hasError || refInvalid} />
      ) : (
        <Input
          id={fieldKey}
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
          autoComplete="off"
          spellCheck={false}
          aria-invalid={hasError || undefined}
          className="pr-9"
        />
      )}
      <div className="absolute right-2">
        <VaultPicker onSelect={onChange} entries={vaultEntries} accountName={account} vaultSettingsUrl={vaultSettingsUrl} />
      </div>
    </div>
  );
}

function SecretField({ fieldKey, meta, value, onChange, hasError, refInvalid, account, vaultEntries, vaultSettingsUrl }: VariableFieldProps) {
  const [revealed, setRevealed] = useState(false);
  const isVaultRef = parseVaultToken(value) !== null;

  return (
    <div className="relative flex items-center">
      {isVaultRef ? (
        <VaultRefChip token={value} onClear={() => onChange("")} invalid={hasError || refInvalid} />
      ) : (
        <Input
          id={fieldKey}
          type={revealed ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
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
        <VaultPicker onSelect={onChange} entries={vaultEntries} accountName={account} vaultSettingsUrl={vaultSettingsUrl} />
      </div>
    </div>
  );
}
