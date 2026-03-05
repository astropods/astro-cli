import Ajv from "ajv";

const ajv = new Ajv({ allErrors: true, verbose: true });

// Map of resource names to their $ref paths in the OpenAPI spec
export const SCHEMA_REFS = {
  MeterCreate: "#/components/schemas/MeterCreate",
  FeatureCreate: "#/components/schemas/FeatureCreateInputs",
  Event: "#/components/schemas/Event",
  EntitlementCreate: "#/components/schemas/EntitlementV2CreateInputs",
  GrantCreate: "#/components/schemas/EntitlementGrantCreateInputV2",
} as const;

/**
 * Resolve a $ref path like "#/components/schemas/Foo" into the actual schema object,
 * recursively resolving nested $refs into a standalone JSON Schema.
 */
export function extractSchema(
  spec: Record<string, unknown>,
  refPath: string
): Record<string, unknown> | null {
  const parts = refPath.replace(/^#\//, "").split("/");
  let node: unknown = spec;
  for (const part of parts) {
    if (node == null || typeof node !== "object") return null;
    node = (node as Record<string, unknown>)[part];
  }
  if (node == null || typeof node !== "object") return null;

  return resolveRefs(spec, node as Record<string, unknown>);
}

function resolveRefs(
  root: Record<string, unknown>,
  schema: Record<string, unknown>
): Record<string, unknown> {
  if ("$ref" in schema && typeof schema["$ref"] === "string") {
    const resolved = extractSchema(root, schema["$ref"]);
    return resolved ?? {};
  }

  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(schema)) {
    if (key === "properties" && value && typeof value === "object") {
      const props: Record<string, unknown> = {};
      for (const [pk, pv] of Object.entries(
        value as Record<string, unknown>
      )) {
        if (pv && typeof pv === "object") {
          props[pk] = resolveRefs(root, pv as Record<string, unknown>);
        } else {
          props[pk] = pv;
        }
      }
      result[key] = props;
    } else if (key === "items" && value && typeof value === "object") {
      result[key] = resolveRefs(root, value as Record<string, unknown>);
    } else if (key === "allOf" || key === "oneOf" || key === "anyOf") {
      if (Array.isArray(value)) {
        result[key] = value.map((v) =>
          v && typeof v === "object"
            ? resolveRefs(root, v as Record<string, unknown>)
            : v
        );
      } else {
        result[key] = value;
      }
    } else if (
      key === "additionalProperties" &&
      value &&
      typeof value === "object"
    ) {
      result[key] = resolveRefs(root, value as Record<string, unknown>);
    } else if (key === "discriminator" && value && typeof value === "object") {
      const disc = value as Record<string, unknown>;
      const resolved: Record<string, unknown> = { ...disc };
      if (disc.mapping && typeof disc.mapping === "object") {
        const mapping: Record<string, unknown> = {};
        for (const [mk, mv] of Object.entries(disc.mapping as Record<string, unknown>)) {
          if (typeof mv === "string") {
            // mv is a $ref string like "#/components/schemas/Foo"
            mapping[mk] = extractSchema(root, mv) ?? {};
          } else if (mv && typeof mv === "object") {
            mapping[mk] = resolveRefs(root, mv as Record<string, unknown>);
          } else {
            mapping[mk] = mv;
          }
        }
        resolved.mapping = mapping;
      }
      result[key] = resolved;
    } else {
      result[key] = value;
    }
  }
  return result;
}

// Cache compiled validators by ref path
const validatorCache = new Map<string, ReturnType<typeof ajv.compile>>();

/**
 * Validate data against a resolved JSON schema.
 */
export function validateAgainstSchema(
  schema: Record<string, unknown> | null,
  data: unknown
): { valid: boolean; errors: ReturnType<typeof ajv.compile>["errors"] } {
  if (!schema) return { valid: false, errors: null };

  const cacheKey = JSON.stringify(schema);
  let validate = validatorCache.get(cacheKey);
  if (!validate) {
    validate = ajv.compile(schema);
    validatorCache.set(cacheKey, validate);
  }

  const valid = validate(data) as boolean;
  return { valid, errors: validate.errors };
}

/**
 * Extract discriminator info from a schema with oneOf + discriminator.
 * Returns the property name and resolved variant schemas keyed by discriminator value.
 */
export function extractDiscriminator(
  schema: Record<string, unknown>
): {
  propertyName: string;
  variants: Record<string, Record<string, unknown>>;
} | null {
  const disc = schema.discriminator as
    | { propertyName?: string; mapping?: Record<string, Record<string, unknown>> }
    | undefined;
  if (!disc?.propertyName || !disc?.mapping) return null;

  const variants: Record<string, Record<string, unknown>> = {};
  for (const [key, variantSchema] of Object.entries(disc.mapping)) {
    if (variantSchema && typeof variantSchema === "object") {
      variants[key] = variantSchema;
    }
  }
  return { propertyName: disc.propertyName, variants };
}

/**
 * Walk schema properties and collect default values into a flat object.
 */
export function getSchemaDefaults(
  schema: Record<string, unknown>
): Record<string, unknown> {
  const defaults: Record<string, unknown> = {};
  const props = schema.properties as Record<string, Record<string, unknown>> | undefined;
  if (!props) return defaults;
  for (const [key, prop] of Object.entries(props)) {
    if (prop && "default" in prop) {
      defaults[key] = prop.default;
    }
  }
  return defaults;
}

export function formatErrors(
  errors: ReturnType<typeof ajv.compile>["errors"]
): string[] {
  if (!errors) return [];
  return errors.map((e) => {
    const path = e.instancePath || "/";
    if (e.keyword === "required")
      return `Missing required field: ${(e.params as { missingProperty: string }).missingProperty}`;
    if (e.keyword === "enum")
      return `${path} must be one of: ${((e.params as { allowedValues: string[] }).allowedValues).join(", ")}`;
    if (e.keyword === "pattern") return `${path} has invalid format`;
    if (e.keyword === "minLength") return `${path} must not be empty`;
    if (e.keyword === "maxLength") return `${path} is too long`;
    if (e.keyword === "minimum")
      return `${path} must be >= ${(e.params as { limit: number }).limit}`;
    if (e.keyword === "additionalProperties")
      return `Unknown field: ${(e.params as { additionalProperty: string }).additionalProperty}`;
    return `${path}: ${e.message}`;
  });
}
