import { describe, it, expect } from "vitest";

import { describeFields, initialFormValue, missingRequired } from "./schema";
import type { JsonSchema } from "./types";

describe("describeFields", () => {
  it("maps each property type to a field kind", () => {
    const schema: JsonSchema = {
      type: "object",
      properties: {
        name: { type: "string" },
        bio: { type: "string", "x-ui": { widget: "textarea" } },
        command: { type: "string", "x-ui": { widget: "code" } },
        age: { type: "number" },
        active: { type: "boolean" },
        env: { type: "string", enum: ["dev", "prod"], enumNames: ["Development", "Production"] },
        regions: { type: "array", items: { type: "string", enum: ["us", "eu"] } },
      },
      required: ["name"],
    };
    const fields = describeFields(schema);
    const byKey = Object.fromEntries(fields.map((f) => [f.key, f]));

    expect(byKey.name.kind).toBe("text");
    expect(byKey.bio.kind).toBe("textarea");
    expect(byKey.command.kind).toBe("code");
    expect(byKey.age.kind).toBe("number");
    expect(byKey.active.kind).toBe("boolean");
    expect(byKey.env.kind).toBe("select");
    expect(byKey.regions.kind).toBe("multiselect");
    expect(byKey.name.required).toBe(true);
    expect(byKey.age.required).toBe(false);
  });

  it("labels from title, else humanizes the key", () => {
    const fields = describeFields({
      type: "object",
      properties: { full_name: { type: "string", title: "Full name" }, apiKey: { type: "string" } },
    });
    expect(fields[0].label).toBe("Full name");
    expect(fields[1].label).toBe("Api Key");
  });

  it("builds select options from enum + enumNames", () => {
    const [env] = describeFields({
      type: "object",
      properties: { env: { type: "string", enum: ["dev", "prod"], enumNames: ["Development", "Production"] } },
    });
    expect(env.options).toEqual([
      { value: "dev", label: "Development", raw: "dev" },
      { value: "prod", label: "Production", raw: "prod" },
    ]);
  });

  it("preserves native enum types for coercion", () => {
    const [count] = describeFields({
      type: "object",
      properties: { count: { type: "integer", enum: [1, 2, 3] } },
    });
    expect(count.options).toEqual([
      { value: "1", label: "1", raw: 1 },
      { value: "2", label: "2", raw: 2 },
      { value: "3", label: "3", raw: 3 },
    ]);
  });

  it("drops an enum member with an empty-string control value", () => {
    const [choice] = describeFields({
      type: "object",
      properties: { choice: { type: "string", enum: ["", "prod"] } },
    });
    expect(choice.options).toEqual([{ value: "prod", label: "prod", raw: "prod" }]);
  });

  it("returns no fields for a non-object schema", () => {
    expect(describeFields({ type: "string" })).toEqual([]);
  });
});

describe("initialFormValue", () => {
  it("prefills from value and fills empties by kind", () => {
    const fields = describeFields({
      type: "object",
      properties: {
        name: { type: "string" },
        active: { type: "boolean" },
        regions: { type: "array", items: { type: "string", enum: ["us"] } },
      },
    });
    const v = initialFormValue(fields, { name: "Mona" });
    expect(v).toEqual({ name: "Mona", active: false, regions: [] });
  });
});

describe("missingRequired", () => {
  const fields = describeFields({
    type: "object",
    properties: {
      name: { type: "string" },
      confirm: { type: "boolean" },
      regions: { type: "array", items: { type: "string", enum: ["us"] } },
    },
    required: ["name", "confirm", "regions"],
  });

  it("flags empty required fields", () => {
    expect(missingRequired(fields, { name: "", confirm: false, regions: [] })).toEqual([
      "name",
      "regions",
    ]);
  });

  it("treats a required boolean=false as answered, not missing", () => {
    // confirm is absent from the missing list above: false is a valid answer.
    expect(missingRequired(fields, { name: "x", confirm: false, regions: ["us"] })).toEqual([]);
  });
});
