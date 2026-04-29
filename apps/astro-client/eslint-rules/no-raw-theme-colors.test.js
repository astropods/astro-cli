import { describe, it } from "vitest";
import { RuleTester } from "eslint";
import rule from "./no-raw-theme-colors.js";

// Hook RuleTester into vitest's describe/it.
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

tester.run("no-raw-theme-colors", rule, {
  valid: [
    // Semantic tokens are always fine.
    { code: 'const x = "bg-card text-card-foreground";' },
    { code: 'const x = "bg-surface text-foreground border border-border";' },
    { code: 'const x = "bg-popover text-muted-foreground";' },

    // Raw colors paired with a `dark:` variant for the same prefix.
    { code: 'const x = "bg-white dark:bg-card";' },
    { code: 'const x = "bg-stone-100 dark:bg-stone-900";' },
    { code: 'const x = "text-stone-600 dark:text-stone-300";' },
    { code: 'const x = "text-green-700 dark:text-green-400";' },
    { code: 'const x = "border-stone-300 dark:border-stone-700";' },

    // Non-color utilities are unaffected.
    { code: 'const x = "p-4 rounded-md flex items-center";' },

    // Template literal with a paired dark variant in the static portion.
    { code: 'const x = `bg-white dark:bg-card ${other}`;' },
  ],

  invalid: [
    {
      code: 'const x = "bg-white";',
      errors: [{ messageId: "rawColor", data: { token: "bg-white", prefix: "bg" } }],
    },
    {
      code: 'const x = "bg-stone-100";',
      errors: [{ messageId: "rawColor" }],
    },
    {
      code: 'const x = "bg-teal-500";',
      errors: [{ messageId: "rawColor" }],
    },
    {
      code: 'const x = "text-stone-600";',
      errors: [{ messageId: "rawColor" }],
    },
    {
      code: 'const x = "text-green-700";',
      errors: [{ messageId: "rawColor" }],
    },
    {
      code: 'const x = "text-coral-600";',
      errors: [{ messageId: "rawColor" }],
    },
    {
      code: 'const x = "border-stone-300";',
      errors: [{ messageId: "rawColor" }],
    },
    {
      // `dark:` modifier is for a different prefix — doesn't satisfy `bg-white`.
      code: 'const x = "bg-white dark:text-card";',
      errors: [{ messageId: "rawColor", data: { token: "bg-white", prefix: "bg" } }],
    },
    {
      // Inside a template literal's static text.
      code: 'const x = `text-stone-600 ${other}`;',
      errors: [{ messageId: "rawColor" }],
    },
    {
      // Inside a JSX className attribute (Literal still visited).
      code: 'const x = <div className="bg-white" />;',
      errors: [{ messageId: "rawColor" }],
    },
    {
      // Two offenders in the same string → two reports.
      code: 'const x = "bg-white text-stone-600";',
      errors: [{ messageId: "rawColor" }, { messageId: "rawColor" }],
    },
  ],
});
