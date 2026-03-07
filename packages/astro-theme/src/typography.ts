/**
 * Typography scale — single source of truth for the Astro design system.
 *
 * Each entry produces a Tailwind v4 composite font-size token:
 *   --text-{name}              → font-size
 *   --text-{name}--line-height → line-height
 *   --text-{name}--letter-spacing → letter-spacing (optional)
 *
 * Usage: `text-heading-1 font-bold`
 */

export interface TypeVariant {
  size: string;
  lineHeight: string;
  letterSpacing?: string;
}

export const typography = {
  display: {
    size: "2.875rem",
    lineHeight: "1.1",
    letterSpacing: "-0.025em",
  },
  "heading-1": {
    size: "1.75rem",
    lineHeight: "1",
    letterSpacing: "-0.02em",
  },
  "heading-2": {
    size: "1.125rem",
    lineHeight: "1",
    letterSpacing: "-0.01em",
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
    size: "0.75rem",
    lineHeight: "1",
    letterSpacing: "0.04em",
  },
  "mono-sm": {
    size: "0.625rem",
    lineHeight: "1",
    letterSpacing: "0.14em",
  },
} as const satisfies Record<string, TypeVariant>;

export type TypeVariantName = keyof typeof typography;
