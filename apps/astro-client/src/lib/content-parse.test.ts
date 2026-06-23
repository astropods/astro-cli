import { describe, it, expect } from "vitest";
import { parseContent, safeStringify } from "./content-parse";

describe("parseContent", () => {
  it("treats objects as JSON without re-parsing", () => {
    const obj = { a: 1, b: [2, 3] };
    const out = parseContent(obj);
    expect(out.isJson).toBe(true);
    expect(out.isEmpty).toBe(false);
    expect(out.json).toBe(obj);
    expect(out.copyText).toBe(JSON.stringify(obj, null, 2));
  });

  it("parses JSON-shaped strings", () => {
    const out = parseContent('{"name": "alice"}');
    expect(out.isJson).toBe(true);
    expect(out.json).toEqual({ name: "alice" });
    expect(out.copyText).toContain('"name"');
  });

  it("parses JSON arrays", () => {
    const out = parseContent("[1,2,3]");
    expect(out.isJson).toBe(true);
    expect(out.json).toEqual([1, 2, 3]);
  });

  it("falls back to text when JSON parse fails", () => {
    const out = parseContent("{not valid json}");
    expect(out.isJson).toBe(false);
    expect(out.text).toBe("{not valid json}");
    expect(out.copyText).toBe("{not valid json}");
  });

  it("treats plain strings as text", () => {
    const out = parseContent("hello world");
    expect(out.isJson).toBe(false);
    expect(out.isEmpty).toBe(false);
    expect(out.text).toBe("hello world");
  });

  it("treats null as empty", () => {
    const out = parseContent(null);
    expect(out.isEmpty).toBe(true);
    expect(out.isJson).toBe(false);
    expect(out.text).toBe("");
  });

  it("treats undefined as empty", () => {
    const out = parseContent(undefined);
    expect(out.isEmpty).toBe(true);
  });

  it("treats empty string as empty", () => {
    const out = parseContent("");
    expect(out.isEmpty).toBe(true);
    expect(out.text).toBe("");
  });
});

describe("safeStringify", () => {
  it("pretty-prints objects with two-space indent", () => {
    expect(safeStringify({ a: 1 })).toBe('{\n  "a": 1\n}');
  });

  it("falls back to String() on cyclic structures", () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;
    expect(safeStringify(cyclic)).toBe(String(cyclic));
  });
});
