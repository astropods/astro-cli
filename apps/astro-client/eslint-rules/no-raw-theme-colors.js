/**
 * Forbid raw Tailwind color utilities in component code in favour of the
 * semantic tokens exposed by `@astropods/theme` (`bg-card`, `bg-surface`,
 * `text-foreground`, `text-muted-foreground`, ...). Semantic tokens flip
 * across light/dark automatically; raw palette utilities do not.
 *
 * Forbidden, when used without a sibling `dark:<same-prefix>` modifier:
 *   bg-white, bg-stone-{n}, bg-teal-{n}
 *   text-{stone|teal|green|coral|yellow|amber|blue}-{n}
 *   border-{stone|teal}-{n}
 *
 * "Sibling" means present in the same className / literal string. Pairing a
 * raw color with `dark:<same-prefix>` (e.g. `bg-white dark:bg-card`) signals
 * an explicit two-mode decision and is allowed.
 *
 * Allowlist (configured in eslint.config.js):
 *   - Storybook story files
 *   - Test files
 *   - A small number of intentionally-literal UI primitives.
 */

const FORBIDDEN_PATTERNS = [
  // bg-white (no scale)
  { regex: /\bbg-white\b/, prefix: "bg" },
  { regex: /\bbg-stone-\d+\b/, prefix: "bg" },
  { regex: /\bbg-teal-\d+\b/, prefix: "bg" },
  // text-{palette}-{scale}
  { regex: /\btext-stone-\d+\b/, prefix: "text" },
  { regex: /\btext-teal-\d+\b/, prefix: "text" },
  { regex: /\btext-green-\d+\b/, prefix: "text" },
  { regex: /\btext-coral-\d+\b/, prefix: "text" },
  { regex: /\btext-yellow-\d+\b/, prefix: "text" },
  { regex: /\btext-amber-\d+\b/, prefix: "text" },
  { regex: /\btext-blue-\d+\b/, prefix: "text" },
  // border-{palette}-{scale}
  { regex: /\bborder-stone-\d+\b/, prefix: "border" },
  { regex: /\bborder-teal-\d+\b/, prefix: "border" },
];

/** Does this class string contain a `dark:` modifier with the matching prefix? */
function hasPairedDark(classString, prefix) {
  return new RegExp(`(?:^|\\s)dark:${prefix}-`).test(classString);
}

/** Walk a class string, return matched tokens that are missing a `dark:` pair. */
function findOffenders(classString) {
  const offenders = [];
  for (const { regex, prefix } of FORBIDDEN_PATTERNS) {
    const match = classString.match(new RegExp(regex.source, "g"));
    if (!match) continue;
    if (hasPairedDark(classString, prefix)) continue;
    for (const token of match) {
      offenders.push({ token, prefix });
    }
  }
  return offenders;
}

const rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "Forbid raw Tailwind palette utilities (bg-white, bg-stone-*, text-stone-*, etc.) without a paired `dark:` variant.",
    },
    messages: {
      rawColor:
        "Raw color utility `{{token}}` requires a sibling `dark:{{prefix}}-…` variant on the same element, or use a semantic token (`bg-card`, `text-muted-foreground`, etc.).",
    },
    schema: [],
  },

  create(context) {
    function check(node, value) {
      if (typeof value !== "string") return;
      const offenders = findOffenders(value);
      for (const { token, prefix } of offenders) {
        context.report({ node, messageId: "rawColor", data: { token, prefix } });
      }
    }

    return {
      Literal(node) {
        check(node, node.value);
      },
      TemplateElement(node) {
        // Only the static text parts of a template literal.
        check(node, node.value.cooked);
      },
    };
  },
};

export default rule;
