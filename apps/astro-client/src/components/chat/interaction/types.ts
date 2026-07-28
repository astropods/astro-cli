// Domain types live in lib/chat/interaction (shared with the data layer),
// re-exported here; this module adds the renderer-only field model.
export type {
  Interaction,
  InteractionAction,
  RenderKind,
  JsonSchema,
  JsonSchemaType,
} from "@/lib/chat/interaction";

export type FieldKind =
  | "text"
  | "textarea"
  | "code"
  | "number"
  | "boolean"
  | "select"
  | "multiselect";

export interface FieldOption {
  // Stringified key for the control (React key, Radix value); `raw` is the native
  // enum member the form submits, so a numeric enum yields numbers not strings.
  value: string;
  label: string;
  raw: unknown;
}

export interface FieldDescriptor {
  key: string;
  label: string;
  description?: string;
  kind: FieldKind;
  required: boolean;
  placeholder?: string;
  options?: FieldOption[];
  min?: number;
  max?: number;
}

// A form's working value, keyed by property (native types).
export type FormValue = Record<string, unknown>;
