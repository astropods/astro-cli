import { useState } from "react";
import { format, parseISO } from "date-fns";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Field, FieldLabel, FieldDescription, FieldGroup } from "@/components/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Calendar } from "@/components/ui/calendar";
import { Plus, Trash2, CalendarIcon, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { extractDiscriminator } from "@/lib/schemas";

type Schema = Record<string, unknown>;

interface SchemaFormProps {
  schema: Schema;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  hiddenFields?: string[];
  defaults?: Record<string, unknown>;
}

export function SchemaForm({ schema, value, onChange, hiddenFields = [], defaults }: SchemaFormProps) {
  const properties = (schema.properties ?? {}) as Record<string, Schema>;
  const required = (schema.required ?? []) as string[];

  const set = (key: string, val: unknown) => {
    onChange({ ...value, [key]: val });
  };

  // Separate simple fields (2-col grid) from full-width fields
  const entries = Object.entries(properties).filter(
    ([key, prop]) => !hiddenFields.includes(key) && !(prop as Schema).deprecated
  );

  const isFullWidth = (prop: Schema): boolean => {
    if (prop.format === "json") return true;
    if (prop.type === "object") return true;
    if (prop.oneOf || prop.anyOf) return true;
    return false;
  };

  const simpleFields = entries.filter(([, p]) => !isFullWidth(p));
  const fullFields = entries.filter(([, p]) => isFullWidth(p));

  return (
    <FieldGroup className="gap-4">
      {simpleFields.length > 0 && (
        <div className="flex flex-col gap-4">
          {simpleFields.map(([key, prop]) => (
            <RenderField
              key={key}
              name={key}
              schema={prop}
              value={value[key]}
              onChange={(v) => set(key, v)}
              required={required.includes(key)}
              defaultValue={defaults?.[key]}
            />
          ))}
        </div>
      )}
      {fullFields.map(([key, prop]) => (
        <RenderField
          key={key}
          name={key}
          schema={prop}
          value={value[key]}
          onChange={(v) => set(key, v)}
          required={required.includes(key)}
          defaultValue={defaults?.[key]}
        />
      ))}
    </FieldGroup>
  );
}

function toLabel(name: string): string {
  return name
    .replace(/([a-z])([A-Z])/g, "$1 $2")
    .replace(/[_-]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

interface RenderFieldProps {
  name: string;
  schema: Schema;
  value: unknown;
  onChange: (value: unknown) => void;
  required?: boolean;
  defaultValue?: unknown;
}

function RenderField({ name, schema, value, onChange, required, defaultValue }: RenderFieldProps) {
  const label = (schema.title as string) ?? toLabel(name);
  const description = schema.description as string | undefined;

  // Enum
  if (schema.enum && Array.isArray(schema.enum)) {
    const items = schema.enum as string[];
    return (
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Select value={String(value ?? defaultValue ?? "")} onValueChange={(v) => onChange(v)}>
          <SelectTrigger size="sm" className="w-full text-[11px]"><SelectValue placeholder={`Select ${label}`} /></SelectTrigger>
          <SelectContent>{items.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent>
        </Select>
        {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      </Field>
    );
  }

  // Boolean — rendered as Select Yes/No
  if (schema.type === "boolean") {
    const boolVal = value === true ? "true" : value === false ? "false" : "";
    return (
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Select value={boolVal} onValueChange={(v) => onChange(v === "true")}>
          <SelectTrigger size="sm" className="w-full text-[11px]"><SelectValue placeholder="Select" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="true">Yes</SelectItem>
            <SelectItem value="false">No</SelectItem>
          </SelectContent>
        </Select>
        {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      </Field>
    );
  }

  // DateTime — Calendar popover + time input
  if (schema.type === "string" && schema.format === "date-time") {
    return (
      <DateTimePicker
        label={label}
        required={required}
        description={description}
        value={value as string | undefined}
        onChange={onChange}
      />
    );
  }

  // JSON format
  if (schema.type === "string" && schema.format === "json") {
    return (
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Textarea
          className="min-h-16 font-mono text-[10px]"
          value={String(value ?? defaultValue ?? "")}
          onChange={(e) => onChange(e.target.value)}
        />
        {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      </Field>
    );
  }

  // Number / integer
  if (schema.type === "number" || schema.type === "integer") {
    return (
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Input
          type="number"
          step={schema.type === "integer" ? "1" : undefined}
          className="h-6"
          value={value != null ? String(value) : String(defaultValue ?? "")}
          onChange={(e) => onChange(e.target.value === "" ? undefined : Number(e.target.value))}
        />
        {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      </Field>
    );
  }

  // Object with properties — nested SchemaForm
  if (schema.type === "object" && schema.properties) {
    const hasFixedProps = Object.keys(schema.properties as Record<string, unknown>).length > 0;
    if (hasFixedProps) {
      return (
        <FieldGroup className="gap-3 rounded-md border border-glass-border-honey p-3">
          <FieldLabel className="text-xs font-medium">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
          <SchemaForm
            schema={schema}
            value={(value ?? {}) as Record<string, unknown>}
            onChange={(v) => onChange(v)}
          />
        </FieldGroup>
      );
    }
  }

  // Object with additionalProperties (no fixed props) — KeyValueEditor
  if (schema.type === "object" && schema.additionalProperties && !schema.properties) {
    return (
      <KeyValueEditor
        label={label}
        required={required}
        description={description}
        value={(value ?? {}) as Record<string, string>}
        onChange={(v) => onChange(Object.keys(v).length > 0 ? v : undefined)}
      />
    );
  }

  // Free-form object (additionalProperties: {} or true, no properties) — raw JSON textarea
  if (schema.type === "object" && !schema.properties && !schema.additionalProperties) {
    return (
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Textarea
          className="min-h-16 font-mono text-[10px]"
          value={typeof value === "string" ? value : JSON.stringify(value ?? {}, null, 2)}
          onChange={(e) => {
            try { onChange(JSON.parse(e.target.value)); } catch { onChange(e.target.value); }
          }}
        />
        {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      </Field>
    );
  }

  // oneOf with discriminator
  if (schema.oneOf && schema.discriminator) {
    return (
      <DiscriminatorField
        name={name}
        schema={schema}
        value={(value ?? {}) as Record<string, unknown>}
        onChange={(v) => onChange(v)}
        required={required}
      />
    );
  }

  // anyOf without discriminator — text input with hint
  if (schema.anyOf && Array.isArray(schema.anyOf)) {
    const enumOptions = (schema.anyOf as Schema[])
      .filter((s) => s.enum)
      .flatMap((s) => (s.enum as string[]) ?? []);
    return (
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Input
          className="h-6"
          value={String(value ?? defaultValue ?? "")}
          onChange={(e) => onChange(e.target.value || undefined)}
        />
        {enumOptions.length > 0 && (
          <FieldDescription className="text-[9px]">Options: {enumOptions.join(", ")}</FieldDescription>
        )}
        {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      </Field>
    );
  }

  // Default: string input
  return (
    <Field className="gap-1.5">
      <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
      <Input
        className="h-6"
        value={String(value ?? defaultValue ?? "")}
        onChange={(e) => onChange(e.target.value || undefined)}
        placeholder={schema.example ? String(schema.example) : undefined}
      />
      {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
    </Field>
  );
}

// KeyValueEditor for additionalProperties objects (metadata, groupBy)
function KeyValueEditor({
  label,
  required,
  description,
  value,
  onChange,
}: {
  label: string;
  required?: boolean;
  description?: string;
  value: Record<string, string>;
  onChange: (value: Record<string, string>) => void;
}) {
  const entries = Object.entries(value);

  const updateEntry = (index: number, key: string, val: string) => {
    const newEntries = [...entries];
    newEntries[index] = [key, val];
    onChange(Object.fromEntries(newEntries));
  };

  const addEntry = () => {
    onChange({ ...value, "": "" });
  };

  const removeEntry = (index: number) => {
    const newEntries = entries.filter((_, i) => i !== index);
    onChange(Object.fromEntries(newEntries));
  };

  return (
    <Field className="gap-1.5">
      <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
      {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
      <div className="space-y-1.5">
        {entries.map(([k, v], i) => (
          <div key={i} className="flex items-center gap-1.5">
            <Input className="h-6" placeholder="key" value={k} onChange={(e) => updateEntry(i, e.target.value, v)} />
            <Input className="h-6" placeholder="value" value={v} onChange={(e) => updateEntry(i, k, e.target.value)} />
            <Button variant="ghost" size="icon-xs" onClick={() => removeEntry(i)}><Trash2 className="size-3 text-red-500" /></Button>
          </div>
        ))}
        <Button variant="outline" size="xs" onClick={addEntry}><Plus className="size-3" /> Add</Button>
      </div>
    </Field>
  );
}

// DiscriminatorField for oneOf with discriminator (e.g., EntitlementV2CreateInputs)
function DiscriminatorField({
  name,
  schema,
  value,
  onChange,
  required,
}: {
  name: string;
  schema: Schema;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  required?: boolean;
}) {
  const disc = extractDiscriminator(schema);
  if (!disc) return null;

  const { propertyName, variants } = disc;
  const currentType = (value[propertyName] as string) ?? "";
  const variantSchema = currentType ? variants[currentType] : null;
  const label = toLabel(name);

  const handleTypeChange = (newType: string) => {
    // Reset to just the discriminator property when switching
    onChange({ [propertyName]: newType });
  };

  return (
    <FieldGroup className="gap-3">
      <Field className="gap-1.5">
        <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
        <Select value={currentType} onValueChange={handleTypeChange}>
          <SelectTrigger size="sm" className="w-full text-[11px]"><SelectValue placeholder={`Select ${toLabel(propertyName)}`} /></SelectTrigger>
          <SelectContent>
            {Object.keys(variants).map((v) => <SelectItem key={v} value={v}>{v}</SelectItem>)}
          </SelectContent>
        </Select>
      </Field>
      {variantSchema && (
        <SchemaForm
          schema={variantSchema}
          value={value}
          onChange={onChange}
          hiddenFields={[propertyName]}
        />
      )}
    </FieldGroup>
  );
}

const HOURS = Array.from({ length: 24 }, (_, i) => String(i).padStart(2, "0"));
const MINUTES = Array.from({ length: 12 }, (_, i) => String(i * 5).padStart(2, "0"));

function DateTimePicker({
  label,
  required,
  description,
  value,
  onChange,
}: {
  label: string;
  required?: boolean;
  description?: string;
  value: string | undefined;
  onChange: (value: unknown) => void;
}) {
  const [open, setOpen] = useState(false);
  const date = value ? parseISO(value) : undefined;
  const hh = date ? String(date.getHours()).padStart(2, "0") : "";
  const mm = date ? String(date.getMinutes()).padStart(2, "0") : "";

  const handleDateSelect = (day: Date | undefined) => {
    if (!day) return;
    const existing = date ?? new Date();
    day.setHours(existing.getHours(), existing.getMinutes(), existing.getSeconds());
    onChange(day.toISOString());
    setOpen(false);
  };

  const setTime = (h: string, m: string) => {
    const d = date ? new Date(date) : new Date();
    d.setHours(Number(h), Number(m), 0);
    onChange(d.toISOString());
  };

  return (
    <Field className="gap-1">
      <FieldLabel className="text-[11px]">{label}{required && <span className="text-destructive"> *</span>}</FieldLabel>
      <div className="flex items-center gap-0.5">
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className={cn("h-6 w-[6.5rem] justify-start gap-0.5 px-1 text-[10px] font-normal", !date && "text-muted-foreground")}
            >
              <CalendarIcon className="size-2 shrink-0 text-muted-foreground" />
              {date ? format(date, "MMM d, yy") : "Pick date"}
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-0" align="start">
            <Calendar mode="single" selected={date} onSelect={handleDateSelect} />
          </PopoverContent>
        </Popover>
        <div className="flex items-center gap-px shrink-0">
          <Select value={hh} onValueChange={(v) => setTime(v, mm || "00")}>
            <SelectTrigger size="sm" className="h-6 w-11 px-1 text-[10px] tabular-nums justify-center"><SelectValue placeholder="HH" /></SelectTrigger>
            <SelectContent className="max-h-48 min-w-0">{HOURS.map((h) => <SelectItem key={h} value={h}>{h}</SelectItem>)}</SelectContent>
          </Select>
          <span className="text-[9px] text-muted-foreground">:</span>
          <Select value={mm} onValueChange={(v) => setTime(hh || "00", v)}>
            <SelectTrigger size="sm" className="h-6 w-11 px-1 text-[10px] tabular-nums justify-center"><SelectValue placeholder="MM" /></SelectTrigger>
            <SelectContent className="max-h-48 min-w-0">{MINUTES.map((m) => <SelectItem key={m} value={m}>{m}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        {date && (
          <Button variant="ghost" size="icon-xs" className="size-5" onClick={() => onChange(undefined)}>
            <X className="size-2.5 text-muted-foreground" />
          </Button>
        )}
      </div>
      {description && <FieldDescription className="text-[9px]">{description}</FieldDescription>}
    </Field>
  );
}
