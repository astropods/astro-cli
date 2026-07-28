// Shared domain types for blocking interactions (the Renderable primitive),
// consumed by the SSE transport, the API/cache layer, and the renderer.

export type InteractionAction = "submit" | "decline" | "cancel" | "respond";

export type RenderKind = "form";

export interface Interaction {
  id: string;
  kind: RenderKind;
  message: string;
  dataSchema: JsonSchema;
  value?: unknown;
  actions: InteractionAction[];
  intent?: string;
}

// The subset of JSON Schema 2020-12 the renderer reads. `enumNames` and `x-ui`
// are the spec's advisory extensions.
export interface JsonSchema {
  type?: JsonSchemaType | JsonSchemaType[];
  properties?: Record<string, JsonSchema>;
  required?: string[];
  enum?: unknown[];
  enumNames?: string[];
  items?: JsonSchema;
  uniqueItems?: boolean;
  format?: string;
  title?: string;
  description?: string;
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  "x-ui"?: { widget?: string; placeholder?: string };
}

export type JsonSchemaType =
  | "object"
  | "array"
  | "string"
  | "number"
  | "integer"
  | "boolean"
  | "null";

export type InteractionResponseBody =
  | { action: "submit"; content: unknown }
  | { action: "respond"; text: string }
  | { action: "decline" | "cancel" };

export interface InteractionResponseAck {
  status: string;
  action: string;
}

// Validate the minimal shape of an interaction payload; null if unusable.
export function parseInteraction(raw: unknown): Interaction | null {
  if (!raw || typeof raw !== "object") return null;
  const o = raw as Record<string, unknown>;
  if (typeof o.id !== "string" || o.id === "") return null;
  if (!o.dataSchema || typeof o.dataSchema !== "object") return null;
  if (!Array.isArray(o.actions)) return null;
  return {
    id: o.id,
    kind: "form",
    message: typeof o.message === "string" ? o.message : "",
    dataSchema: o.dataSchema as JsonSchema,
    value: o.value,
    actions: o.actions.filter((a): a is InteractionAction =>
      a === "submit" || a === "decline" || a === "cancel" || a === "respond",
    ),
    intent: typeof o.intent === "string" ? o.intent : undefined,
  };
}
