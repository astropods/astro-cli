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
