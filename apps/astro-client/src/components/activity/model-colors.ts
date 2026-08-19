const MODEL_DISPLAY_NAMES: Record<string, string> = {
  "claude-opus-4-7": "Claude Opus 4.7",
  "claude-sonnet-4-6": "Claude Sonnet 4.6",
  "claude-sonnet-4-5-20250929": "Claude Sonnet 4.5",
  "claude-haiku-4-5-20251001": "Claude Haiku 4.5",
  "gpt-4o-2024-08-06": "GPT-4o",
  "gpt-4o-mini-2024-07-18": "GPT-4o mini",
  "gpt-4-turbo": "GPT-4 Turbo",
  "gpt-4": "GPT-4",
  "o1": "o1",
  "o3": "o3",
  "o3-mini": "o3 mini",
};

/** Maps a raw model API ID to a human-readable display name, stripping date suffixes as fallback. */
export function formatModelName(id: string): string {
  return MODEL_DISPLAY_NAMES[id] ?? id.replace(/-\d{8}$/, "");
}

// Fixed palette for model colors in charts — consistent mapping by index.
// Using CSS vars from the theme palette (same approach as chart-utils.ts).
const MODEL_PALETTE = [
  "var(--color-indigo-500)",
  "var(--color-teal-400)",
  "var(--color-blue-600)",
  "var(--color-indigo-300)",
  "var(--color-teal-600)",
  "var(--color-blue-400)",
  "var(--color-indigo-700)",
  "var(--color-teal-500)",
];

export function modelColor(index: number): string {
  return MODEL_PALETTE[index % MODEL_PALETTE.length];
}

export function buildModelColorMap(models: string[]): Record<string, string> {
  const map: Record<string, string> = {};
  models.forEach((m, i) => { map[m] = modelColor(i); });
  return map;
}

// Distinct palette for dev-tool sources in charts — visually separate from the
// agent MODEL_PALETTE.
const DEVTOOL_PALETTE = [
  "var(--color-amber-500)",
  "var(--color-purple-500)",
  "var(--color-pink-500)",
  "var(--color-green-500)",
];

export function devtoolColor(index: number): string {
  return DEVTOOL_PALETTE[index % DEVTOOL_PALETTE.length];
}

// Brand presentation for known dev-tool sources: chart color + integration-icon
// key. Unknown sources fall back to DEVTOOL_PALETTE with no logo.
export const SOURCE_BRAND: Record<string, { color: string; icon?: string }> = {
  "claude-code": { color: "var(--color-amber-500)", icon: "claude-code" },
  codex: { color: "var(--color-foreground)", icon: "openai" },
};

export function devtoolSourceColor(key: string, index: number): string {
  return SOURCE_BRAND[key]?.color ?? devtoolColor(index);
}

// One palette per axis: each axis's first label is slot 0, so a shared palette
// could not colour them independently. Saturated shades only — the neutral
// families read as disabled, and the -300 tints wash out.
//
// Index is a label's declared slot, so reordering an entry repaints a category.
const PURPOSE_PALETTE = [
  "var(--color-indigo-500)", // 0 work
  "var(--color-yellow-400)", // 1 personal
  "var(--color-teal-500)",   // 2 ambiguous
  "var(--color-green-700)",  // 3 overflow — a label this build does not know
];

const TOPIC_PALETTE = [
  "var(--color-yellow-400)", // 0  software-engineering
  "var(--color-indigo-500)", // 1  data-analytics
  "var(--color-teal-500)",   // 2  product
  "var(--color-amber-500)",  // 3  design
  "var(--color-blue-500)",   // 4  marketing
  "var(--color-green-600)",  // 5  sales
  "var(--color-pink-500)",   // 6  customer-support
  "var(--color-red-500)",    // 7  operations-it
  "var(--color-purple-500)", // 8  hr-recruiting
  "var(--color-teal-700)",   // 9  finance-legal
  "var(--color-green-400)",  // 10 research-learning
  "var(--color-blue-700)",   // 11 creative-writing
  "var(--color-teal-400)",   // 12 general-knowledge
  "var(--color-purple-700)", // 13 personal-life
  "var(--color-pink-700)",   // 14 other
  "var(--color-green-700)",  // 15 overflow — a label this build does not know
];

const AXIS_PALETTES: Record<string, string[]> = {
  purpose: PURPOSE_PALETTE,
  topic: TOPIC_PALETTE,
};

/** Keyed by axis and declared slot, not position in a response.
 *
 *  Clamped, not wrapped: the server sends an overflow slot past the declared
 *  labels for anything it does not know, and a modulo would fold that back onto
 *  a real category — painting an unknown label as software engineering. */
export function categoryColor(axisKey: string, colorIndex: number): string {
  const palette = AXIS_PALETTES[axisKey] ?? TOPIC_PALETTE;
  const i = Math.min(Math.max(colorIndex, 0), palette.length - 1);
  return palette[i];
}
