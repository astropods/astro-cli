import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";
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
import type { VariableDisplay } from "./VariableFields";

/** Convert "SLACK_BOT_TOKEN" → "your-slack-bot-token" */
function placeholderFromKey(key: string): string {
  return "your-" + key.replace(/_/g, "-").toLowerCase();
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
}

export function VariableField({ fieldKey, meta, value, onChange, hasError }: VariableFieldProps) {
  // 1. Select dropdown
  if (meta.displayAs === "select" && meta.options && meta.options.length > 0) {
    return (
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger
          id={fieldKey}
          className="bg-white"
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
        className="bg-white"
        autoComplete="off"
        spellCheck={false}
        aria-invalid={hasError || undefined}
      />
    );
  }

  // 3. Boolean toggle
  if (meta.datatype === "boolean") {
    return (
      <div className="flex items-center gap-3 pt-1">
        <Switch
          id={fieldKey}
          checked={value === "true"}
          onCheckedChange={(checked) => onChange(checked ? "true" : "false")}
        />
        <label
          htmlFor={fieldKey}
          className="text-sm text-muted-foreground select-none cursor-pointer"
        >
          {value === "true" ? "Enabled" : "Disabled"}
        </label>
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
        className="bg-white"
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
        className="bg-white"
        autoComplete="off"
        spellCheck={false}
        aria-invalid={hasError || undefined}
      />
    );
  }

  // 6. Secret (password) input with reveal toggle
  if (meta.secret) {
    return <SecretField fieldKey={fieldKey} meta={meta} value={value} onChange={onChange} hasError={hasError} />;
  }

  // 7. Default — text input
  return (
    <Input
      id={fieldKey}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
      className="bg-white"
      autoComplete="off"
      spellCheck={false}
      aria-invalid={hasError || undefined}
    />
  );
}

function SecretField({ fieldKey, meta, value, onChange, hasError }: VariableFieldProps) {
  const [revealed, setRevealed] = useState(false);

  return (
    <div className="relative">
      <Input
        id={fieldKey}
        type={revealed ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={meta.placeholder || placeholderFromKey(fieldKey)}
        className="bg-white pr-9"
        autoComplete="off"
        spellCheck={false}
        aria-invalid={hasError || undefined}
      />
      <button
        type="button"
        onClick={() => setRevealed((r) => !r)}
        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
        aria-label={revealed ? "Hide value" : "Reveal value"}
      >
        {revealed ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}
