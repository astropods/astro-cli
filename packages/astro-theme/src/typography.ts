/**
 * Typography scale — single source of truth for the Astro design system.
 *
 * Each entry produces a Tailwind v4 composite font-size token:
 *   --text-{name}              → font-size
 *   --text-{name}--line-height → line-height
 *   --text-{name}--letter-spacing → letter-spacing (optional)
 *   --text-{name}--font-weight   → font-weight (optional)
 *
 * Usage: `text-heading-1` (includes size, line-height, and weight)
 */

export interface TypeVariant {
  size: string;
  lineHeight: string;
  letterSpacing?: string;
  fontWeight?: string;
}

export const typography = {
  display: {
    size: "2.875rem",
    lineHeight: "1.1",
    letterSpacing: "-0.025em",
    fontWeight: "600",
  },
  "heading-1": {
    size: "1.75rem",
    lineHeight: "1",
    letterSpacing: "-0.02em",
    fontWeight: "600",
  },
  "heading-2": {
    size: "1.125rem",
    lineHeight: "1",
    letterSpacing: "-0.01em",
    fontWeight: "600",
  },
  "heading-3": {
    size: "1rem",
    lineHeight: "1",
    letterSpacing: "-0.01em",
    fontWeight: "600",
  },
  "heading-4": {
    size: "0.875rem",
    lineHeight: "1",
    fontWeight: "600",
  },
  body: {
    size: "0.875rem",
    lineHeight: "1.65",
  },
  "body-sm": {
    size: "0.75rem",
    lineHeight: "1.55",
  },
  label: {
    size: "0.6875rem",
    lineHeight: "1",
    letterSpacing: "0.18em",
  },
  "mono-md": {
    size: "0.875rem",
    lineHeight: "1",
    letterSpacing: "0.02em",
  },
  "mono-sm": {
    size: "0.75rem",
    lineHeight: "1",
    letterSpacing: "0.07em",
  },
  "mono-xs": {
    size: "0.6875rem",
    lineHeight: "1rem",
    letterSpacing: "0",
  },
} as const satisfies Record<string, TypeVariant>;

export type TypeVariantName = keyof typeof typography;
