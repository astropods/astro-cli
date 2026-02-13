/**
 * Color palette derived from the astro-client tailwind theme (replace-colors branch).
 * Each scale goes from 50 (lightest) to 950 (darkest).
 * Hex values are pre-converted from the OKLCH originals for portability.
 */

export type ColorScale = {
  50: string;
  100: string;
  200: string;
  300: string;
  400: string;
  500: string;
  600: string;
  700: string;
  800: string;
  900: string;
  950: string;
};

export const indigo: ColorScale = {
  50: "#eef2fe",
  100: "#e0e7fd",
  200: "#c7d3fb",
  300: "#a3b5fa",
  400: "#7a8bf7",
  500: "#5d67f2",
  600: "#4948e6",
  700: "#3e3bc9",
  800: "#3333a0",
  900: "#2e317d",
  950: "#1d1d48",
};

export const neutral: ColorScale = {
  50: "#fafafa",
  100: "#f4f4f4",
  200: "#e7e7e7",
  300: "#d4d4d4",
  400: "#a0a0a0",
  500: "#727272",
  600: "#545454",
  700: "#414141",
  800: "#292929",
  900: "#191919",
  950: "#070707",
};

export const stone: ColorScale = {
  50: "#f9fbf8",
  100: "#f6f5f1",
  200: "#e9e5dd",
  300: "#d8d4c5",
  400: "#a9a188",
  500: "#7b7356",
  600: "#56563d",
  700: "#44422c",
  800: "#2f2517",
  900: "#1e1a0c",
  950: "#0e0a03",
};

export const red: ColorScale = {
  50: "#fdf2f3",
  100: "#fde3e3",
  200: "#fccbcc",
  300: "#fca5a8",
  400: "#f86c73",
  500: "#f1404b",
  600: "#de2031",
  700: "#b9192d",
  800: "#981c24",
  900: "#7d2025",
  950: "#430d0f",
};

export const amber: ColorScale = {
  50: "#fef7f0",
  100: "#feeddb",
  200: "#fbd7b6",
  300: "#faba86",
  400: "#f59055",
  500: "#f37644",
  600: "#e35d3d",
  700: "#bb4733",
  800: "#933e1a",
  900: "#753327",
  950: "#3f1812",
};

export const yellow: ColorScale = {
  50: "#fefbf0",
  100: "#f9f3d9",
  200: "#f6e6b1",
  300: "#f2d273",
  400: "#e7b726",
  500: "#dc9f00",
  600: "#c78500",
  700: "#9e6500",
  800: "#7f5200",
  900: "#654300",
  950: "#362200",
};

export const green: ColorScale = {
  50: "#f2fcf4",
  100: "#e0fbe6",
  200: "#c2f5ce",
  300: "#91eda5",
  400: "#58da70",
  500: "#3fc44f",
  600: "#31a23d",
  700: "#2b7f34",
  800: "#24642f",
  900: "#21522a",
  950: "#0e2d15",
};

export const teal: ColorScale = {
  50: "#f3fcfb",
  100: "#d7faf5",
  200: "#b1f2e8",
  300: "#81e6d9",
  400: "#46c6ba",
  500: "#26a59b",
  600: "#15827d",
  700: "#106763",
  800: "#064944",
  900: "#073d3c",
  950: "#002323",
};

export const blue: ColorScale = {
  50: "#eff5fe",
  100: "#dceafc",
  200: "#c0dafb",
  300: "#92c3fc",
  400: "#59a1fa",
  500: "#3380f7",
  600: "#1861ec",
  700: "#0053d7",
  800: "#1840ab",
  900: "#1d3b85",
  950: "#162550",
};

export const purple: ColorScale = {
  50: "#f9f5ff",
  100: "#f1e8ff",
  200: "#e6d6ff",
  300: "#d4b4ff",
  400: "#b97eff",
  500: "#a34cff",
  600: "#8e1dff",
  700: "#790ae1",
  800: "#6718b6",
  900: "#541a90",
  950: "#37066a",
};

export const pink: ColorScale = {
  50: "#fcf2f9",
  100: "#fbe7f5",
  200: "#facfec",
  300: "#faa6dc",
  400: "#f766c1",
  500: "#f236a7",
  600: "#e20084",
  700: "#c40068",
  800: "#a10056",
  900: "#84124a",
  950: "#500529",
};

/** All available color palettes keyed by name. */
export const palettes = {
  indigo,
  neutral,
  stone,
  red,
  amber,
  yellow,
  green,
  teal,
  blue,
  purple,
  pink,
} as const;

export type PaletteName = keyof typeof palettes;

/** Flat list of palette names for iteration. */
export const paletteNames: PaletteName[] = Object.keys(palettes) as PaletteName[];
