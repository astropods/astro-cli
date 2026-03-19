/**
 * Avatar content for the card. Either raw SVG markup or an image URL.
 * - `{ svg: "<rect ...>" }` — inline SVG content (no outer `<svg>` wrapper).
 * - `{ url: "https://..." }` — image URL (rendered via `<image>`).
 */
export type CardAvatar =
  | { svg: string; url?: never }
  | { url: string; svg?: never };

/** Card color scheme derived from avatar colors. */
export interface CardColors {
  /** Dark background color — the source color darkened to ~10-15% lightness. */
  background: string;
  /** Light foreground/text color — the source color lightened to ~90-95% lightness. */
  foreground: string;
  /** Vibrant accent color — the most saturated dominant color from the avatar. */
  accent: string;
  /** Lighter accent variant for secondary elements. */
  accentLight?: string;
  /** High lightness, still saturated version of the accent — for labels and barcode. */
  glow?: string;
}

/** CardColors with all optional fields resolved. Used internally by renderers. */
export type ResolvedCardColors = Required<CardColors>;

/** A user account displayed inline in a stat row. */
export interface CardAccount {
  fullName: string;
  handle: string;
  avatarUrl?: string | null;
}

/** A label-value row displayed in the stats section. */
export type CardStat =
  | { label: string; value: string; account?: never }
  | { label: string; account: CardAccount; value?: never };

/** An integration displayed as a pill badge. */
export interface CardIntegration {
  /** Display name (e.g. "Slack"). */
  name: string;
  /** SVG icon content (no outer <svg> wrapper, assumes 24x24 coordinate space). */
  icon?: string;
  /** URL to an icon image (used if `icon` is not provided). */
  iconUrl?: string;
}

/** Data used to populate a trading card. */
export interface CardData {
  /** Agent slug/identifier (e.g. "my-agent"). */
  name: string;
  /** Human-readable display name (e.g. "Research Assistant"). Falls back to name. */
  displayName?: string;
  /** Account/org that owns the agent. */
  account: string;
  /** Agent avatar — inline SVG content or an image URL. */
  avatar?: CardAvatar;
  /** Colors derived from the avatar, provided by the caller. */
  colors?: CardColors;
  /** Short description of the agent. */
  description?: string;
  /** Tags/categories. */
  tags?: string[];
  /** Number of hearts/likes. */
  heartCount?: number;
  /** Label-value rows for the stats section. */
  stats?: CardStat[];
  /** Integrations to display as pills with icons. */
  integrations?: CardIntegration[];
  /** ID string to render as a barcode at the bottom of the card. */
  barcodeId?: string;
  /** URL to encode as a QR code displayed beside the barcode. */
  qrUrl?: string;
}

/** Dimensions for a card variant in pixels. */
export interface CardDimensions {
  width: number;
  height: number;
}

export const CARD_DIMENSIONS = {
  standard: { width: 350, height: 560 },
} as const;
