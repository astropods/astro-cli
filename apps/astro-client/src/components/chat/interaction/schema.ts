import type {
  FieldDescriptor,
  FieldOption,
  FormValue,
  JsonSchema,
  JsonSchemaType,
} from "./types";

function hasType(schema: JsonSchema, t: JsonSchemaType): boolean {
  return Array.isArray(schema.type) ? schema.type.includes(t) : schema.type === t;
}

// snake_case / camelCase → friendly label: "set_invoices" → "Set invoices".
export function humanize(key: string): string {
  const spaced = key.replace(/[_-]+/g, " ").replace(/([a-z0-9])([A-Z])/g, "$1 $2");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function optionsFrom(schema: JsonSchema): FieldOption[] {
  if (!schema.enum) return [];
  return schema.enum
    .map((v, i) => ({
      value: String(v),
      label: schema.enumNames?.[i] ?? String(v),
      raw: v,
    }))
    // Radix Select reserves "" as a value; drop a member that stringifies to it.
    .filter((o) => o.value !== "");
}

function fieldKind(prop: JsonSchema): FieldDescriptor["kind"] {
  if (prop.enum) return "select";
  if (hasType(prop, "array") && prop.items?.enum) return "multiselect";
  if (hasType(prop, "boolean")) return "boolean";
  if (hasType(prop, "number") || hasType(prop, "integer")) return "number";
  if (prop["x-ui"]?.widget === "code") return "code";
  if (prop["x-ui"]?.widget === "textarea") return "textarea";
  return "text";
}

// Flatten an object schema into ordered field descriptors (non-object → none).
export function describeFields(schema: JsonSchema): FieldDescriptor[] {
  if (!schema.properties) return [];
  const required = new Set(schema.required ?? []);

  return Object.entries(schema.properties).map(([key, prop]) => {
    const kind = fieldKind(prop);
    const options =
      kind === "multiselect" ? optionsFrom(prop.items ?? {}) : kind === "select" ? optionsFrom(prop) : undefined;
    return {
      key,
      label: prop.title ?? humanize(key),
      description: prop.description,
      kind,
      required: required.has(key),
      placeholder: prop["x-ui"]?.placeholder,
      options,
      min: prop.minimum,
      max: prop.maximum,
    };
  });
}

// Seed the form from any prefilled `value`, defaulting unset fields by kind.
export function initialFormValue(fields: FieldDescriptor[], value: unknown): FormValue {
  const prefill = (value && typeof value === "object" ? value : {}) as Record<string, unknown>;
  const out: FormValue = {};
  for (const field of fields) {
    if (field.key in prefill) {
      out[field.key] = prefill[field.key];
      continue;
    }
    out[field.key] = field.kind === "multiselect" ? [] : field.kind === "boolean" ? false : undefined;
  }
  return out;
}

// Required fields left empty — the gate for enabling submit.
export function missingRequired(fields: FieldDescriptor[], value: FormValue): string[] {
  return fields
    .filter((f) => f.required && isEmpty(value[f.key]))
    .map((f) => f.key);
}

function isEmpty(v: unknown): boolean {
  if (v === undefined || v === null || v === "") return true;
  if (Array.isArray(v)) return v.length === 0;
  return false;
}
