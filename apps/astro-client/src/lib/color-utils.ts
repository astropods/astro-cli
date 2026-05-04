/** Convert a hex color (#RRGGBB) to hue (0–360), saturation (0–100), and lightness (0–100). */
export function hexToHSL(hex: string) {
  const r = parseInt(hex.slice(1, 3), 16) / 255;
  const g = parseInt(hex.slice(3, 5), 16) / 255;
  const b = parseInt(hex.slice(5, 7), 16) / 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  let h = 0;
  let s = 0;
  const l = (max + min) / 2;
  if (max !== min) {
    const d = max - min;
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break;
      case g: h = ((b - r) / d + 2) / 6; break;
      default: h = ((r - g) / d + 4) / 6; break;
    }
  }
  return {
    hue: Math.round(h * 360),
    saturation: Math.round(s * 100),
    lightness: Math.round(l * 100),
  };
}

/**
 * Derive accent colors from a hex color string.
 * Converts to HSL, clamps saturation to a usable range, and returns
 * light and dark variants suitable for interactive elements.
 */
export function deriveAccentColors(hex: string) {
  const { hue, saturation } = hexToHSL(hex);
  const ctaSat = Math.min(75, Math.max(35, saturation));
  return {
    light: `hsl(${hue} ${ctaSat}% 45%)`,
    dark: `hsl(${hue} ${ctaSat}% 32%)`,
    hue,
    saturation: ctaSat,
  };
}
