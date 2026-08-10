import { lazy, Suspense, useState, useEffect, useRef, useCallback } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { AvatarColors } from "@/lib/api";
import { HoloButton } from "@/components/ui/holo-button";
import { ArrowRight } from "lucide-react";
import { extractPalette, pickCardColors, parseHex } from "astro-trading-card";

// --- Color utilities (mirror server's colorextract) ---

function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255;
  const max = Math.max(r, g, b), min = Math.min(r, g, b);
  const l = (max + min) / 2;
  if (max === min) return [0, 0, l];
  const d = max - min;
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
  let h = 0;
  if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
  else if (max === g) h = ((b - r) / d + 2) / 6;
  else h = ((r - g) / d + 4) / 6;
  return [h * 360, s, l];
}

function hslToHex(h: number, s: number, l: number): string {
  h /= 360;
  const hue2rgb = (p: number, q: number, t: number) => {
    if (t < 0) t += 1; if (t > 1) t -= 1;
    if (t < 1 / 6) return p + (q - p) * 6 * t;
    if (t < 1 / 2) return q;
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6;
    return p;
  };
  let r: number, g: number, b: number;
  if (s === 0) { r = g = b = l; } else {
    const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
    const p = 2 * l - q;
    r = hue2rgb(p, q, h + 1 / 3); g = hue2rgb(p, q, h); b = hue2rgb(p, q, h - 1 / 3);
  }
  return `#${[r, g, b].map((c) => Math.round(c * 255).toString(16).padStart(2, "0")).join("")}`;
}

function deriveAvatarColors(cardColors: { accent: string; accentLight: string; background: string; foreground: string; glow: string }): AvatarColors {
  const rgb = parseHex(cardColors.accent);
  if (!rgb) {
    return {
      base: cardColors.accent, vibrant: cardColors.accent, vibrant_light: cardColors.accent,
      accent: cardColors.accent, accent_light: cardColors.accentLight,
      background: cardColors.background, foreground: cardColors.foreground, glow: cardColors.glow,
    };
  }
  const [h, s, l] = rgbToHsl(rgb.r, rgb.g, rgb.b);
  return {
    base: hslToHex(h, s * 0.5, l),
    vibrant: hslToHex(h, Math.min(s, 0.5), 0.35),
    vibrant_light: hslToHex(h, Math.min(s, 0.6), 0.7),
    accent: cardColors.accent,
    accent_light: cardColors.accentLight,
    background: cardColors.background,
    foreground: cardColors.foreground,
    glow: cardColors.glow,
  };
}

interface ExtractionResult {
  colors: AvatarColors;
  palette: Array<{ hex: string; population: number; r: number; g: number; b: number }>;
}

function extractFromImage(img: HTMLImageElement): ExtractionResult | null {
  const canvas = document.createElement("canvas");
  const size = 64;
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.drawImage(img, 0, 0, size, size);
  const { data } = ctx.getImageData(0, 0, size, size);
  const palette = extractPalette(data, 8, 5);
  const card = pickCardColors(palette);
  if (!card) return null;

  return {
    colors: deriveAvatarColors(card as { accent: string; accentLight: string; background: string; foreground: string; glow: string }),
    palette: palette.map((s) => ({ hex: s.hex, population: s.population, r: s.r, g: s.g, b: s.b })),
  };
}

// --- UI components ---

// The committed placeholder avatars — the only avatar images every checkout
// has (assets/avatars/ is per-developer and gitignored), and varied enough to
// exercise the extractor.
const AVATAR_URLS = Array.from({ length: 25 }, (_, i) =>
  `/assets/placeholders/accounts/avatar_${String(i + 1).padStart(2, "0")}.jpg`
);

function ColorSwatch({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex flex-col items-center gap-0.5">
      <div className="size-5 rounded-sm border border-white/20" style={{ backgroundColor: color }} />
      <span className="text-[8px] text-muted-foreground leading-tight">{label}</span>
      <span className="font-mono text-[7px] text-muted-foreground/60 leading-tight">{color}</span>
    </div>
  );
}

function GradientWashPreview({ colors, avatarUrl }: { colors: AvatarColors; avatarUrl: string }) {
  const id = `diag-grid-${colors.accent.replace("#", "")}`;
  return (
    <div className="relative h-full w-full overflow-hidden rounded-[4px] border border-border bg-surface">
      <div className="pointer-events-none absolute inset-0 [mask-image:radial-gradient(ellipse_80%_120%_at_25%_0%,black_0%,transparent_70%)]">
        <div
          className="absolute inset-0 dark:hidden"
          style={{ background: `radial-gradient(ellipse 80% 70% at 25% 0%, color-mix(in oklch, ${colors.glow} 25%, transparent) 0%, transparent 80%)` }}
        />
        <div
          className="absolute inset-0 hidden dark:block"
          style={{ background: `radial-gradient(ellipse 80% 70% at 25% 0%, color-mix(in oklch, ${colors.base} 24%, transparent) 0%, transparent 80%)` }}
        />
        <svg className="absolute inset-0 h-full w-full dark:hidden" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <pattern id={`${id}-l`} width="8" height="8" patternUnits="userSpaceOnUse">
              <path d="M 8 0 L 0 0 0 8" fill="none" stroke={colors.vibrant_light} strokeWidth="0.75" strokeOpacity="0.35" />
            </pattern>
          </defs>
          <rect width="100%" height="100%" fill={`url(#${id}-l)`} />
        </svg>
        <svg className="absolute inset-0 hidden h-full w-full dark:block" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <pattern id={`${id}-d`} width="8" height="8" patternUnits="userSpaceOnUse">
              <path d="M 8 0 L 0 0 0 8" fill="none" stroke="white" strokeWidth="0.5" strokeOpacity="0.12" />
            </pattern>
          </defs>
          <rect width="100%" height="100%" fill={`url(#${id}-d)`} />
        </svg>
      </div>
      <div className="relative z-[1] flex items-center gap-3 p-4">
        <img src={avatarUrl} alt="" className="size-10 rounded-sm border border-border object-cover" />
        <div>
          <div className="text-sm font-semibold text-foreground">sample-agent</div>
          <div className="font-mono text-xs text-muted-foreground">account/sample-agent</div>
        </div>
      </div>
    </div>
  );
}

function AvatarTile({ url }: { url: string }) {
  const [result, setResult] = useState<ExtractionResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const imgRef = useRef<HTMLImageElement>(null);

  const onLoad = useCallback(() => {
    const img = imgRef.current;
    if (!img) return;
    try {
      const r = extractFromImage(img);
      if (r) setResult(r);
      else setError("No colors extracted");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Extraction failed");
    }
  }, []);

  const colors = result?.colors ?? null;

  return (
    <div className="flex gap-4 rounded-lg border border-border bg-background p-4">
      <div className="w-[160px] shrink-0">
        <img
          ref={imgRef}
          src={url}
          crossOrigin="anonymous"
          onLoad={onLoad}
          onError={() => setError("Image failed to load")}
          alt=""
          className="size-16 rounded-md border border-border mb-2 object-cover"
        />
        {colors ? (
          <div className="grid grid-cols-4 gap-1.5">
            <ColorSwatch color={colors.accent} label="accent" />
            <ColorSwatch color={colors.base} label="base" />
            <ColorSwatch color={colors.glow} label="glow" />
            <ColorSwatch color={colors.vibrant} label="vibrant" />
            <ColorSwatch color={colors.vibrant_light} label="vib_lt" />
            <ColorSwatch color={colors.accent_light} label="acc_lt" />
            <ColorSwatch color={colors.background} label="bg" />
            <ColorSwatch color={colors.foreground} label="fg" />
          </div>
        ) : error ? (
          <p className="text-xs text-red-500">{error}</p>
        ) : (
          <p className="text-xs text-muted-foreground">Extracting...</p>
        )}
      </div>

      <div className="flex-1 min-w-0 flex flex-col">
        <p className="text-[10px] font-mono text-muted-foreground mb-1.5 uppercase tracking-wider">Gradient wash</p>
        <div className="flex-1 min-h-[100px]">
          {colors ? <GradientWashPreview colors={colors} avatarUrl={url} /> : <div className="h-full rounded-[4px] border border-border bg-muted/20 animate-pulse" />}
        </div>
      </div>

      <div className="w-[320px] shrink-0 flex flex-col">
        <p className="text-[10px] font-mono text-muted-foreground mb-1.5 uppercase tracking-wider">HoloButton</p>
        <div className="flex flex-1 items-center justify-center rounded-[4px] border border-border bg-stone-200 p-4 dark:bg-muted/30">
          <HoloButton accentHex={colors?.accent} size="default" className="h-11 w-full">
            Deploy
            <ArrowRight className="h-4 w-4" />
          </HoloButton>
        </div>
      </div>
    </div>
  );
}

function ColorDiagnosticsPanel() {
  const [customUrls, setCustomUrls] = useState<string[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFiles = useCallback((files: FileList | null) => {
    if (!files) return;
    for (const file of Array.from(files)) {
      if (!file.type.startsWith("image/")) continue;
      setCustomUrls((prev) => [...prev, URL.createObjectURL(file)]);
    }
  }, []);

  useEffect(() => {
    return () => customUrls.forEach((u) => { if (u.startsWith("blob:")) URL.revokeObjectURL(u); });
  }, [customUrls]);

  return (
    <div className="flex flex-col gap-4 p-6">
      <div className="flex items-center justify-between mb-2">
        <div>
          <h2 className="text-xl font-bold text-foreground">Color Diagnostics</h2>
          <p className="text-body-sm text-muted-foreground">
            Real avatar images extracted via MMCQ then run through the full AvatarColors derivation pipeline.
          </p>
        </div>
        <div>
          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            multiple
            className="hidden"
            onChange={(e) => handleFiles(e.target.files)}
          />
          <button
            type="button"
            className="rounded-md border border-border bg-surface px-3 py-1.5 text-sm text-foreground hover:bg-muted/30 transition-colors"
            onClick={() => fileInputRef.current?.click()}
          >
            + Add custom image
          </button>
        </div>
      </div>

      {customUrls.map((url) => (
        <AvatarTile key={url} url={url} />
      ))}

      {AVATAR_URLS.map((url) => (
        <AvatarTile key={url} url={url} />
      ))}
    </div>
  );
}

// --- Color relationship chart ---

export interface ColorEntry {
  label: string;
  hex: string;
  h: number;
  s: number;
  l: number;
  group: "extracted" | "derived" | "button";
  note?: string;
}

function buildColorEntries(colors: AvatarColors): ColorEntry[] {
  const toHsl = (hex: string) => {
    const rgb = parseHex(hex);
    if (!rgb) return { h: 0, s: 0, l: 0 };
    const [h, s, l] = rgbToHsl(rgb.r, rgb.g, rgb.b);
    return { h, s, l };
  };

  // Derive the button colors the same way HoloButton does internally
  const accentRgb = parseHex(colors.accent);
  let btnHue = 0;
  let btnSat = 0.35;
  if (accentRgb) {
    const [h, s] = rgbToHsl(accentRgb.r, accentRgb.g, accentRgb.b);
    btnHue = Math.round(h);
    btnSat = Math.min(0.75, Math.max(0.35, s));
  }
  const btnLightHex = hslToHex(btnHue, btnSat, 0.45);
  const btnDarkHex = hslToHex(btnHue, btnSat, 0.32);

  const entry = (label: string, hex: string, group: ColorEntry["group"], note?: string): ColorEntry => {
    const { h, s, l } = toHsl(hex);
    return { label, hex, h, s, l, group, note };
  };

  return [
    entry("accent", colors.accent, "extracted", "Raw dominant color from MMCQ"),
    entry("accent_light", colors.accent_light, "extracted", "Secondary color, s≤0.6, l=0.75"),
    entry("base", colors.base, "derived", "accent hue, s×0.5, original l"),
    entry("vibrant", colors.vibrant, "derived", "accent hue, s≤0.5, l=0.35"),
    entry("vibrant_light", colors.vibrant_light, "derived", "accent hue, s≤0.6, l=0.7"),
    entry("glow", colors.glow, "derived", "accent hue, s≤0.9, l=0.8"),
    entry("background", colors.background, "derived", "accent hue, s≤0.5, l=0.09"),
    entry("foreground", colors.foreground, "derived", "accent hue, s≤0.1, l=0.96"),
    entry("btn light", btnLightHex, "button", `hue=${btnHue}, s=clamp(35-75%)=${Math.round(btnSat * 100)}%, l=45%`),
    entry("btn dark", btnDarkHex, "button", `hue=${btnHue}, s=clamp(35-75%)=${Math.round(btnSat * 100)}%, l=32%`),
  ];
}


// Lazy-load Three.js dependencies so they're only pulled into Storybook, never the production site
const ThreeScene = lazy(() => import("./ColorDiagnostics3D"));

function ColorRelationshipChart({ colors }: { colors: AvatarColors }) {
  const entries = buildColorEntries(colors);

  return (
    <div>
      <p className="text-xs font-mono text-muted-foreground mb-2 uppercase tracking-wider">Color relationships (3D HSL cylinder — drag to rotate)</p>
      <div className="rounded-[4px] border border-border bg-background p-6">
        <div className="flex gap-8 items-start">
          <div className="shrink-0 rounded-[4px] overflow-hidden" style={{ width: 520, height: 480 }}>
            <Suspense fallback={<div className="flex items-center justify-center h-full text-muted-foreground text-sm">Loading 3D...</div>}>
              <ThreeScene entries={entries} />
            </Suspense>
          </div>

          {/* Side table */}
          <div className="flex-1 min-w-0">
            <table className="w-full text-[10px] font-mono border-collapse">
              <thead>
                <tr className="text-muted-foreground border-b border-border">
                  <th className="text-left pb-2 font-normal" />
                  <th className="text-left pb-2 font-normal">Name</th>
                  <th className="text-left pb-2 font-normal">Hex</th>
                  <th className="text-right pb-2 font-normal">H</th>
                  <th className="text-right pb-2 font-normal">S</th>
                  <th className="text-right pb-2 font-normal">L</th>
                  <th className="text-left pb-2 font-normal pl-3">Transform</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry, i) => {
                  const prevGroup = i > 0 ? entries[i - 1].group : null;
                  const showDivider = entry.group !== prevGroup && i > 0;
                  return (
                    <tr key={i} className={showDivider ? "border-t-2 border-border" : "border-t border-border/30"}>
                      <td className="py-1.5 pr-2">
                        <div className="size-4 rounded-sm border border-white/20" style={{ backgroundColor: entry.hex }} />
                      </td>
                      <td className="py-1.5 pr-2">
                        <span className="text-foreground">{entry.label}</span>
                      </td>
                      <td className="py-1.5 text-muted-foreground pr-2">{entry.hex}</td>
                      <td className="py-1.5 text-right text-muted-foreground tabular-nums">{Math.round(entry.h)}</td>
                      <td className="py-1.5 text-right text-muted-foreground tabular-nums">{Math.round(entry.s * 100)}%</td>
                      <td className="py-1.5 text-right text-muted-foreground tabular-nums">{Math.round(entry.l * 100)}%</td>
                      <td className="py-1.5 pl-3 text-muted-foreground/70 text-[9px]">{entry.note}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}

/** 2D chart focused on the accent → button color transformation. */
function ButtonDerivationChart({ colors }: { colors: AvatarColors }) {
  const accentRgb = parseHex(colors.accent);
  if (!accentRgb) return null;

  const [accentH, accentS, accentL] = rgbToHsl(accentRgb.r, accentRgb.g, accentRgb.b);
  const btnHue = Math.round(accentH);
  const btnSat = Math.min(0.75, Math.max(0.35, accentS));

  // All the steps in the transformation
  const steps = [
    { label: "accent (input)", s: accentS, l: accentL, hex: colors.accent, note: "Raw extracted color" },
    { label: "vibrant", s: Math.min(accentS, 0.5), l: 0.35, hex: colors.vibrant, note: "s capped 50%, l forced 35%" },
    { label: "base", s: accentS * 0.5, l: accentL, hex: colors.base, note: "s halved, l unchanged" },
    { label: "btn light", s: btnSat, l: 0.45, hex: hslToHex(btnHue, btnSat, 0.45), note: `s clamped 35-75%=${Math.round(btnSat * 100)}%, l=45%` },
    { label: "btn dark", s: btnSat, l: 0.32, hex: hslToHex(btnHue, btnSat, 0.32), note: `s clamped 35-75%=${Math.round(btnSat * 100)}%, l=32%` },
  ];

  // Chart dimensions
  const w = 500;
  const h = 300;
  const pad = { top: 30, right: 20, bottom: 40, left: 50 };
  const plotW = w - pad.left - pad.right;
  const plotH = h - pad.top - pad.bottom;

  // X = saturation (0-1), Y = lightness (0-1)
  const toX = (s: number) => pad.left + s * plotW;
  const toY = (l: number) => pad.top + plotH - l * plotH;

  return (
    <div>
      <p className="text-xs font-mono text-muted-foreground mb-2 uppercase tracking-wider">
        Button color derivation (Hue {btnHue}° — Saturation vs Lightness)
      </p>
      <div className="rounded-[4px] border border-border bg-background p-4">
        <svg width={w} height={h} className="overflow-visible">
          {/* Background: render a saturation × lightness gradient at the fixed hue */}
          {Array.from({ length: 20 }, (_, si) =>
            Array.from({ length: 20 }, (_, li) => {
              const s0 = si / 20, s1 = (si + 1) / 20;
              const l0 = li / 20, l1 = (li + 1) / 20;
              const sm = (s0 + s1) / 2, lm = (l0 + l1) / 2;
              return (
                <rect
                  key={`${si}-${li}`}
                  x={toX(s0)} y={toY(l1)}
                  width={plotW / 20} height={plotH / 20}
                  fill={hslToHex(btnHue, sm, lm)}
                  opacity={0.15}
                />
              );
            })
          )}

          {/* Axes */}
          <line x1={pad.left} y1={toY(0)} x2={pad.left + plotW} y2={toY(0)} stroke="currentColor" strokeOpacity="0.15" />
          <line x1={pad.left} y1={toY(0)} x2={pad.left} y2={toY(1)} stroke="currentColor" strokeOpacity="0.15" />

          {/* Grid */}
          {[0.25, 0.5, 0.75, 1].map((v) => (
            <g key={`grid-${v}`}>
              <line x1={toX(v)} y1={toY(0)} x2={toX(v)} y2={toY(1)} stroke="currentColor" strokeOpacity="0.06" strokeDasharray="2 4" />
              <line x1={toX(0)} y1={toY(v)} x2={toX(1)} y2={toY(v)} stroke="currentColor" strokeOpacity="0.06" strokeDasharray="2 4" />
            </g>
          ))}

          {/* X axis labels */}
          {[0, 25, 50, 75, 100].map((v) => (
            <text key={`x-${v}`} x={toX(v / 100)} y={toY(0) + 16} textAnchor="middle" className="fill-muted-foreground text-[9px]">{v}%</text>
          ))}
          <text x={pad.left + plotW / 2} y={h - 2} textAnchor="middle" className="fill-muted-foreground text-[9px]">Saturation</text>

          {/* Y axis labels */}
          {[0, 25, 50, 75, 100].map((v) => (
            <text key={`y-${v}`} x={pad.left - 8} y={toY(v / 100) + 3} textAnchor="end" className="fill-muted-foreground text-[9px]">{v}%</text>
          ))}
          <text x={12} y={pad.top + plotH / 2} textAnchor="middle" className="fill-muted-foreground text-[9px]" transform={`rotate(-90, 12, ${pad.top + plotH / 2})`}>Lightness</text>

          {/* Arrows showing transformation from accent to each derived color */}
          {steps.slice(1).map((step, i) => {
            const accent = steps[0];
            return (
              <line key={`arrow-${i}`}
                x1={toX(accent.s)} y1={toY(accent.l)}
                x2={toX(step.s)} y2={toY(step.l)}
                stroke="currentColor" strokeOpacity="0.25" strokeWidth="1"
                markerEnd="url(#arrowhead)"
              />
            );
          })}

          {/* Arrowhead marker */}
          <defs>
            <marker id="arrowhead" markerWidth="6" markerHeight="4" refX="5" refY="2" orient="auto">
              <polygon points="0 0, 6 2, 0 4" fill="currentColor" fillOpacity="0.25" />
            </marker>
          </defs>

          {/* Data points */}
          {steps.map((step, i) => {
            const x = toX(step.s);
            const y = toY(step.l);
            const isAccent = i === 0;
            return (
              <g key={i}>
                <circle cx={x} cy={y} r={isAccent ? 8 : 6} fill={step.hex} stroke="white" strokeWidth="2" />
                <text x={x} y={y - (isAccent ? 12 : 10)} textAnchor="middle" className="fill-foreground text-[9px] font-mono" style={{ paintOrder: "stroke", stroke: "var(--color-background)", strokeWidth: 3 }}>
                  {step.label}
                </text>
                <text x={x} y={y + (isAccent ? 18 : 16)} textAnchor="middle" className="fill-muted-foreground text-[7px] font-mono" style={{ paintOrder: "stroke", stroke: "var(--color-background)", strokeWidth: 3 }}>
                  S:{Math.round(step.s * 100)}% L:{Math.round(step.l * 100)}%
                </text>
              </g>
            );
          })}
        </svg>

        {/* Inline legend */}
        <div className="mt-3 pt-3 border-t border-border flex gap-6 text-[10px] font-mono text-muted-foreground">
          <span>Input: <strong className="text-foreground">accent</strong> H:{btnHue}° S:{Math.round(accentS * 100)}% L:{Math.round(accentL * 100)}%</span>
          <span>HoloButton clamps S to 35-75% (actual: {Math.round(btnSat * 100)}%)</span>
        </div>
      </div>
    </div>
  );
}

// --- Storybook config ---

const meta = {
  title: "Diagnostics/ColorDiagnostics",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => <ColorDiagnosticsPanel />,
};

function SingleDiagnosticPanel() {
  const [activeUrl, setActiveUrl] = useState(AVATAR_URLS[0]);
  const [result, setResult] = useState<ExtractionResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const imgRef = useRef<HTMLImageElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [customUrl, setCustomUrl] = useState<string | null>(null);

  const colors = result?.colors ?? null;

  const extract = useCallback(() => {
    const img = imgRef.current;
    if (!img) return;
    setError(null);
    try {
      const r = extractFromImage(img);
      if (r) setResult(r);
      else setError("No colors extracted");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Extraction failed");
    }
  }, []);

  const handleUpload = useCallback((files: FileList | null) => {
    if (!files?.[0]) return;
    const url = URL.createObjectURL(files[0]);
    if (customUrl) URL.revokeObjectURL(customUrl);
    setCustomUrl(url);
    setActiveUrl(url);
    setResult(null);
  }, [customUrl]);

  useEffect(() => {
    return () => { if (customUrl) URL.revokeObjectURL(customUrl); };
  }, [customUrl]);

  const id = colors ? `single-grid-${colors.accent.replace("#", "")}` : "single-grid";

  return (
    <div className="flex flex-col gap-6 p-8 max-w-[1200px] mx-auto">
      {/* Controls */}
      <div className="flex items-center gap-4">
        <select
          className="rounded-md border border-border bg-surface px-3 py-2 text-sm text-foreground"
          value={activeUrl}
          onChange={(e) => { setActiveUrl(e.target.value); setResult(null); }}
        >
          {AVATAR_URLS.map((url, i) => (
            <option key={url} value={url}>Image {i + 1}</option>
          ))}
          {customUrl && <option value={customUrl}>Custom upload</option>}
        </select>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(e) => handleUpload(e.target.files)}
        />
        <button
          type="button"
          className="rounded-md border border-border bg-surface px-3 py-2 text-sm text-foreground hover:bg-muted/30 transition-colors"
          onClick={() => fileInputRef.current?.click()}
        >
          Upload image
        </button>
      </div>

      {/* Hidden image for extraction */}
      <img
        ref={imgRef}
        key={activeUrl}
        src={activeUrl}
        crossOrigin="anonymous"
        onLoad={extract}
        onError={() => setError("Image failed to load")}
        alt=""
        className="hidden"
      />

      <div className="flex gap-6">
        {/* Left: avatar + swatches */}
        <div className="w-[220px] shrink-0 flex flex-col gap-4">
          <img
            src={activeUrl}
            alt=""
            className="size-32 rounded-lg border border-border object-cover"
          />
          {result ? (
            <>
              {/* Raw MMCQ palette */}
              <div>
                <p className="text-[9px] font-mono text-muted-foreground mb-1.5 uppercase tracking-wider">MMCQ palette</p>
                <div className="flex flex-col gap-1">
                  {result.palette.map((swatch, i) => {
                    const isAccent = swatch.hex === colors!.accent;
                    const isAccentLight = swatch.hex === colors!.accent_light || (colors!.accent_light && parseHex(colors!.accent_light) && (() => {
                      const [sh] = rgbToHsl(swatch.r, swatch.g, swatch.b);
                      const alRgb = parseHex(colors!.accent_light);
                      if (!alRgb) return false;
                      const [ah] = rgbToHsl(alRgb.r, alRgb.g, alRgb.b);
                      return Math.abs(sh - ah) < 5;
                    })());
                    const totalPop = result.palette.reduce((s, p) => s + p.population, 0);
                    const popPct = ((swatch.population / totalPop) * 100).toFixed(1);
                    const [, sat, lum] = rgbToHsl(swatch.r, swatch.g, swatch.b);
                    return (
                      <div key={i} className="flex items-center gap-1.5">
                        <div className="size-4 rounded-sm shrink-0" style={{ backgroundColor: swatch.hex, outline: isAccent ? "2px solid white" : "none" }} />
                        <div className="flex-1 h-2 rounded-full bg-muted/30 overflow-hidden">
                          <div className="h-full rounded-full" style={{ width: `${popPct}%`, backgroundColor: swatch.hex }} />
                        </div>
                        <span className="text-[8px] font-mono text-muted-foreground w-[28px] text-right">{popPct}%</span>
                        <span className="text-[8px] font-mono text-muted-foreground w-[28px] text-right">S:{Math.round(sat * 100)}</span>
                        <span className="text-[8px] font-mono text-muted-foreground w-[28px] text-right">L:{Math.round(lum * 100)}</span>
                        {isAccent && <span className="text-[7px] font-mono text-foreground">← vibrant</span>}
                        {isAccentLight && !isAccent && <span className="text-[7px] font-mono text-muted-foreground">← lt vibrant</span>}
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* Derived theme */}
              <div>
                <p className="text-[9px] font-mono text-muted-foreground mb-1.5 uppercase tracking-wider">Derived theme</p>
                <div className="grid grid-cols-4 gap-3">
                  <ColorSwatch color={colors!.accent} label="accent" />
                  <ColorSwatch color={colors!.base} label="base" />
                  <ColorSwatch color={colors!.glow} label="glow" />
                  <ColorSwatch color={colors!.vibrant} label="vibrant" />
                  <ColorSwatch color={colors!.vibrant_light} label="vib_lt" />
                  <ColorSwatch color={colors!.accent_light} label="acc_lt" />
                  <ColorSwatch color={colors!.background} label="bg" />
                  <ColorSwatch color={colors!.foreground} label="fg" />
                </div>
              </div>
            </>
          ) : error ? (
            <p className="text-sm text-red-500">{error}</p>
          ) : (
            <p className="text-sm text-muted-foreground">Extracting...</p>
          )}
        </div>

        {/* Right: previews */}
        <div className="flex-1 min-w-0 flex flex-col gap-6">
          {/* Gradient wash — large */}
          <div>
            <p className="text-xs font-mono text-muted-foreground mb-2 uppercase tracking-wider">Gradient wash</p>
            <div className="relative h-[240px] w-full overflow-hidden rounded-[4px] border border-border bg-surface">
              {colors && (
                <div className="pointer-events-none absolute inset-0 [mask-image:radial-gradient(ellipse_80%_120%_at_25%_0%,black_0%,transparent_70%)]">
                  <div
                    className="absolute inset-0 dark:hidden"
                    style={{ background: `radial-gradient(ellipse 80% 70% at 25% 0%, color-mix(in oklch, ${colors.glow} 25%, transparent) 0%, transparent 80%)` }}
                  />
                  <div
                    className="absolute inset-0 hidden dark:block"
                    style={{ background: `radial-gradient(ellipse 80% 70% at 25% 0%, color-mix(in oklch, ${colors.base} 24%, transparent) 0%, transparent 80%)` }}
                  />
                  <svg className="absolute inset-0 h-full w-full dark:hidden" xmlns="http://www.w3.org/2000/svg">
                    <defs>
                      <pattern id={`${id}-l`} width="8" height="8" patternUnits="userSpaceOnUse">
                        <path d="M 8 0 L 0 0 0 8" fill="none" stroke={colors.vibrant_light} strokeWidth="0.75" strokeOpacity="0.35" />
                      </pattern>
                    </defs>
                    <rect width="100%" height="100%" fill={`url(#${id}-l)`} />
                  </svg>
                  <svg className="absolute inset-0 hidden h-full w-full dark:block" xmlns="http://www.w3.org/2000/svg">
                    <defs>
                      <pattern id={`${id}-d`} width="8" height="8" patternUnits="userSpaceOnUse">
                        <path d="M 8 0 L 0 0 0 8" fill="none" stroke="white" strokeWidth="0.5" strokeOpacity="0.12" />
                      </pattern>
                    </defs>
                    <rect width="100%" height="100%" fill={`url(#${id}-d)`} />
                  </svg>
                </div>
              )}
              <div className="relative z-[1] flex items-center gap-4 p-6">
                <img src={activeUrl} alt="" className="size-14 rounded-sm border border-border object-cover" />
                <div>
                  <div className="text-lg font-semibold text-foreground">sample-agent</div>
                  <div className="font-mono text-sm text-muted-foreground">account/sample-agent</div>
                </div>
              </div>
            </div>
          </div>

          {/* Color relationship chart */}
          {colors && <ColorRelationshipChart colors={colors} />}

          {/* Button derivation chart */}
          {colors && <ButtonDerivationChart colors={colors} />}

          {/* HoloButton — large */}
          <div>
            <p className="text-xs font-mono text-muted-foreground mb-2 uppercase tracking-wider">HoloButton</p>
            <div className="flex items-center justify-center rounded-[4px] border border-border bg-stone-200 p-6 dark:bg-muted/30">
              <HoloButton accentHex={colors?.accent} size="default" className="h-12 w-full max-w-[400px] text-base">
                Deploy this agent
                <ArrowRight className="h-5 w-5" />
              </HoloButton>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export const Single: Story = {
  render: () => <SingleDiagnosticPanel />,
};
