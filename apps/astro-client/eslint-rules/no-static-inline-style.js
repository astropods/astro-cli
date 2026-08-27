/**
 * Forbid a `style={{ ... }}` property whose value is a plain string/number
 * literal, for a small allowlist of CSS properties Tailwind unambiguously
 * covers (`display`, `width`, `height`, `color`, `background`,
 * `backgroundColor`, `flexGrow`, `fontSize`, ...). A literal value here has
 * no runtime reason not to be a Tailwind class.
 *
 * Deliberately narrow. Most `style={{}}` usage in this codebase is a value
 * the Tailwind JIT scanner can't statically resolve into a class (a color
 * computed from a JS variable, an SVG transform/mask, a measured pixel
 * offset, an animation name) — that's the legitimate case the two named
 * exceptions in `apps/astro-client/CLAUDE.md` describe, and this rule
 * doesn't touch it: only a `Literal` AST node on a property in the
 * allowlist below is flagged, never an identifier, member expression,
 * template literal, or call expression. Extend the allowlist only for a
 * property Tailwind definitely has a utility for; when in doubt, leave it
 * off rather than risk a false positive on a genuinely inexpressible value.
 */

const FLAGGED_PROPS = new Set([
  "display",
  "width",
  "height",
  "color",
  "background",
  "backgroundColor",
  "flexGrow",
  "fontSize",
]);

// A string literal that calls a CSS function is a computed/theme-driven
// value, not a lazy static one, even though the AST sees a plain string:
// `var(--success)`, `color-mix(in oklch, ...)`, `linear-gradient(...)`,
// `calc(...)`, `clamp(...)`. Skip these rather than flag them.
const DYNAMIC_CSS_FUNCTION = /\b(?:var|calc|clamp|min|max|color-mix|(?:repeating-)?(?:linear|radial|conic)-gradient)\(/;

const rule = {
  meta: {
    type: "problem",
    docs: {
      description:
        "Forbid a literal-valued style={{}} property for CSS properties Tailwind already covers.",
    },
    messages: {
      staticInlineStyle:
        "`{{prop}}: {{value}}` is a literal value with a Tailwind equivalent — use a className instead of style={{}}.",
    },
    schema: [],
  },

  create(context) {
    return {
      JSXAttribute(node) {
        if (node.name.name !== "style") return;
        const expr = node.value?.expression;
        if (expr?.type !== "ObjectExpression") return;

        for (const prop of expr.properties) {
          if (prop.type !== "Property") continue;
          const key = prop.key.name ?? prop.key.value;
          if (!FLAGGED_PROPS.has(key)) continue;
          if (prop.value.type !== "Literal") continue;
          if (
            typeof prop.value.value === "string" &&
            DYNAMIC_CSS_FUNCTION.test(prop.value.value)
          ) {
            continue;
          }

          context.report({
            node: prop,
            messageId: "staticInlineStyle",
            data: { prop: key, value: String(prop.value.value) },
          });
        }
      },
    };
  },
};

export default rule;
