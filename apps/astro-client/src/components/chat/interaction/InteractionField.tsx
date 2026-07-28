import { CheckIcon } from "lucide-react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

import type { FieldDescriptor, FieldOption } from "./types";

interface InteractionFieldProps {
  field: FieldDescriptor;
  value: unknown;
  onChange: (value: unknown) => void;
  invalid?: boolean;
  disabled?: boolean;
}

export function InteractionField({ field, value, onChange, invalid, disabled }: InteractionFieldProps) {
  const describedBy = field.description ? `${field.key}-desc` : undefined;

  return (
    <div className="min-w-0">
      <div className="mb-1.5 flex items-baseline gap-1">
        <Label size="md" htmlFor={field.key}>
          {field.label}
        </Label>
        {field.required && <span className="text-destructive">*</span>}
      </div>
      {field.description && (
        <p id={describedBy} className="mb-1.5 text-body-sm text-muted-foreground">
          {field.description}
        </p>
      )}
      <FieldControl
        field={field}
        value={value}
        onChange={onChange}
        invalid={invalid}
        disabled={disabled}
        describedBy={describedBy}
      />
    </div>
  );
}

function FieldControl({
  field,
  value,
  onChange,
  invalid,
  disabled,
  describedBy,
}: InteractionFieldProps & { describedBy?: string }) {
  switch (field.kind) {
    case "textarea":
      return (
        <Textarea
          id={field.key}
          value={(value as string) ?? ""}
          placeholder={field.placeholder}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
        />
      );

    case "code":
      return (
        <Textarea
          id={field.key}
          rows={commandRows(value)}
          spellCheck={false}
          value={(value as string) ?? ""}
          placeholder={field.placeholder}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          disabled={disabled}
          className="resize-none bg-surface font-mono text-mono-sm leading-relaxed"
          onChange={(e) => onChange(e.target.value)}
        />
      );

    case "number":
      return (
        <Input
          id={field.key}
          type="number"
          value={value === undefined || value === null ? "" : String(value)}
          placeholder={field.placeholder}
          min={field.min}
          max={field.max}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          disabled={disabled}
          onChange={(e) => {
            const n = e.target.value === "" ? undefined : Number(e.target.value);
            onChange(n !== undefined && Number.isNaN(n) ? undefined : n);
          }}
        />
      );

    case "boolean":
      return (
        <div className="flex items-center gap-2">
          <Switch
            id={field.key}
            checked={Boolean(value)}
            disabled={disabled}
            aria-describedby={describedBy}
            onCheckedChange={(checked) => onChange(checked)}
          />
        </div>
      );

    case "select": {
      const options = field.options ?? [];
      return (
        <Select
          value={value === undefined || value === null ? undefined : String(value)}
          disabled={disabled}
          onValueChange={(v) => onChange(options.find((o) => o.value === v)?.raw ?? v)}
        >
          <SelectTrigger id={field.key} aria-invalid={invalid || undefined} aria-describedby={describedBy}>
            <SelectValue placeholder={field.placeholder ?? "Select…"} />
          </SelectTrigger>
          <SelectContent>
            {options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      );
    }

    case "multiselect":
      return (
        <CheckboxList
          options={field.options ?? []}
          value={Array.isArray(value) ? value : []}
          disabled={disabled}
          describedBy={describedBy}
          onChange={onChange}
        />
      );

    default:
      return (
        <Input
          id={field.key}
          value={(value as string) ?? ""}
          placeholder={field.placeholder}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
        />
      );
  }
}

// Size the code block to its content, capped, so it's neither dwarfed nor huge.
function commandRows(value: unknown): number {
  const lines = typeof value === "string" && value ? value.split("\n").length : 1;
  return Math.min(Math.max(lines, 1), 12);
}

interface CheckboxListProps {
  options: FieldOption[];
  value: unknown[];
  disabled?: boolean;
  describedBy?: string;
  onChange: (value: unknown[]) => void;
}

function CheckboxList({ options, value, disabled, describedBy, onChange }: CheckboxListProps) {
  const isChecked = (o: FieldOption) => value.some((x) => String(x) === o.value);
  const toggle = (o: FieldOption) =>
    onChange(isChecked(o) ? value.filter((x) => String(x) !== o.value) : [...value, o.raw]);

  return (
    <div role="group" aria-describedby={describedBy} className="flex flex-col gap-1">
      {options.map((o) => {
        const checked = isChecked(o);
        return (
          <button
            key={o.value}
            type="button"
            disabled={disabled}
            onClick={() => toggle(o)}
            className="flex w-full items-center gap-2 rounded-sm px-1 py-1.5 text-left text-body text-foreground transition-colors hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
          >
            <span
              className={cn(
                "flex size-4 shrink-0 items-center justify-center rounded-xs border transition-colors",
                checked ? "border-primary bg-primary text-primary-foreground" : "border-border",
              )}
            >
              {checked && <CheckIcon className="size-3" />}
            </span>
            {o.label}
          </button>
        );
      })}
    </div>
  );
}
