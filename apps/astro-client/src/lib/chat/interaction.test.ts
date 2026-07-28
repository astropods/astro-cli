import { describe, it, expect } from "vitest";

import { parseInteraction } from "./interaction";

describe("parseInteraction", () => {
  const valid = {
    type: "interaction",
    id: "i1",
    kind: "form",
    message: "pick one",
    dataSchema: { type: "object", properties: { x: { type: "string" } } },
    value: { x: "y" },
    actions: ["submit", "cancel"],
    intent: "tool_permission",
  };

  it("parses a well-formed interaction", () => {
    const it_ = parseInteraction(valid);
    expect(it_).not.toBeNull();
    expect(it_).toMatchObject({
      id: "i1",
      kind: "form",
      message: "pick one",
      actions: ["submit", "cancel"],
      intent: "tool_permission",
    });
    expect(it_!.dataSchema).toEqual(valid.dataSchema);
    expect(it_!.value).toEqual({ x: "y" });
  });

  it("defaults message to empty and intent to undefined when absent", () => {
    const it_ = parseInteraction({ ...valid, message: undefined, intent: undefined });
    expect(it_!.message).toBe("");
    expect(it_!.intent).toBeUndefined();
  });

  it("drops unknown actions", () => {
    const it_ = parseInteraction({ ...valid, actions: ["submit", "bogus", "respond"] });
    expect(it_!.actions).toEqual(["submit", "respond"]);
  });

  it.each([
    ["missing id", { ...valid, id: undefined }],
    ["empty id", { ...valid, id: "" }],
    ["non-object dataSchema", { ...valid, dataSchema: "nope" }],
    ["non-array actions", { ...valid, actions: "submit" }],
    ["not an object", 42],
    ["null", null],
  ])("returns null for %s", (_label, input) => {
    expect(parseInteraction(input)).toBeNull();
  });
});
