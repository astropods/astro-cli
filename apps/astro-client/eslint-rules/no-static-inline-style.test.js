import { describe, it } from "vitest";
import { RuleTester } from "eslint";
import rule from "./no-static-inline-style.js";

RuleTester.describe = describe;
RuleTester.it = it;
RuleTester.itOnly = it.only;

const tester = new RuleTester({
  languageOptions: {
    parserOptions: {
      ecmaVersion: "latest",
      sourceType: "module",
      ecmaFeatures: { jsx: true },
    },
  },
});

tester.run("no-static-inline-style", rule, {
  valid: [
    // A dynamic value (identifier) on a flagged property is legitimate.
    { code: "const x = <div style={{ background: color }} />;" },
    { code: "const x = <div style={{ backgroundColor: bgColor }} />;" },
    { code: "const x = <div style={{ color: toneConfig.textColor }} />;" },

    // A template literal is legitimate (runtime-computed).
    { code: "const x = <div style={{ width: `${w}px` }} />;" },

    // A call expression is legitimate.
    { code: "const x = <div style={{ height: getHeight() }} />;" },

    // A literal value on a property NOT in the allowlist is untouched
    // (SVG/animation/mask properties with no Tailwind equivalent).
    { code: 'const x = <div style={{ transformBox: "fill-box" }} />;' },
    { code: 'const x = <div style={{ maskType: "alpha" }} />;' },
    { code: 'const x = <div style={{ animation: "float 3s" }} />;' },
    { code: 'const x = <div style={{ mixBlendMode: "screen" }} />;' },

    // No style attribute at all.
    { code: 'const x = <div className="flex" />;' },

    // style prop present but not an object literal (e.g. spread var).
    { code: "const x = <div style={computedStyle} />;" },

    // A string literal calling a dynamic CSS function is computed/theme-
    // driven, not a lazy static value, even though the AST sees a string.
    { code: 'const x = <div style={{ color: "var(--card-contrast)" }} />;' },
    { code: 'const x = <div style={{ background: "var(--success)" }} />;' },
    {
      code: 'const x = <div style={{ background: "color-mix(in oklch, var(--color-yellow-500) 12%, transparent)" }} />;',
    },
    {
      code: 'const x = <div style={{ background: "linear-gradient(to bottom, var(--color-background), transparent)" }} />;',
    },
    { code: 'const x = <div style={{ width: "calc(100% - 8px)" }} />;' },
    { code: 'const x = <div style={{ fontSize: "clamp(14px, 2vw, 18px)" }} />;' },
  ],

  invalid: [
    {
      code: 'const x = <div style={{ display: "flex" }} />;',
      errors: [{ messageId: "staticInlineStyle", data: { prop: "display", value: "flex" } }],
    },
    {
      code: 'const x = <div style={{ width: "100px" }} />;',
      errors: [{ messageId: "staticInlineStyle", data: { prop: "width", value: "100px" } }],
    },
    {
      code: "const x = <div style={{ height: 100 }} />;",
      errors: [{ messageId: "staticInlineStyle", data: { prop: "height", value: "100" } }],
    },
    {
      code: 'const x = <div style={{ color: "red" }} />;',
      errors: [{ messageId: "staticInlineStyle" }],
    },
    {
      code: 'const x = <div style={{ fontSize: "14px" }} />;',
      errors: [{ messageId: "staticInlineStyle" }],
    },
    {
      code: "const x = <div style={{ flexGrow: 1 }} />;",
      errors: [{ messageId: "staticInlineStyle" }],
    },
    // Mixed object: only the literal, flagged property is reported.
    {
      code: 'const x = <div style={{ display: "flex", transform: `translateY(${y}px)` }} />;',
      errors: [{ messageId: "staticInlineStyle", data: { prop: "display", value: "flex" } }],
    },
  ],
});
