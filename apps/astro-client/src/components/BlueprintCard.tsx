import { useState, useCallback, useMemo } from "react";
import { Link } from "react-router";
import { EllipsisHorizontalIcon, ArchiveBoxIcon, ArrowRightIcon } from "@heroicons/react/24/outline";
import { BlueprintIdentity } from "./BlueprintIdentity";
import { UserAvatar } from "./UserAvatar";
import { PrivacyBadge } from "@/components/PrivacyBadge";
import { StatusBadge } from "@/components/StatusBadge";
import { ArchiveBlueprintDialog } from "@/components/ArchiveBlueprintDialog";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { extractPalette, pickCardColors, parseHex } from "astro-trading-card";

interface CardAccent {
  base: string;
  vibrant: string;
}

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
  return [h, s, l];
}

function hslToHex(h: number, s: number, l: number): string {
  const hue2rgb = (p: number, q: number, t: number) => {
    if (t < 0) t += 1; if (t > 1) t -= 1;
    if (t < 1/6) return p + (q - p) * 6 * t;
    if (t < 1/2) return q;
    if (t < 2/3) return p + (q - p) * (2/3 - t) * 6;
    return p;
  };
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
  const p = 2 * l - q;
  const r = Math.round(hue2rgb(p, q, h + 1/3) * 255);
  const g = Math.round(hue2rgb(p, q, h) * 255);
  const b = Math.round(hue2rgb(p, q, h - 1/3) * 255);
  return `#${((1 << 24) + (r << 16) + (g << 8) + b).toString(16).slice(1)}`;
}

/** Extract accent colors directly from an already-loaded <img> element. */
function extractAccentFromImg(img: HTMLImageElement): CardAccent | null {
  try {
    const canvas = document.createElement("canvas");
    canvas.width = 64;
    canvas.height = 64;
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.drawImage(img, 0, 0, 64, 64);
    const { data } = ctx.getImageData(0, 0, 64, 64);
    const palette = extractPalette(data, 8);
    const colors = pickCardColors(palette);
    if (!colors) return null;
    const rgb = parseHex(colors.accent);
    if (!rgb) return { base: colors.accent, vibrant: colors.accent };
    const [h, s] = rgbToHsl(rgb.r, rgb.g, rgb.b);
    return {
      base: hslToHex(h, s * 0.5, rgbToHsl(rgb.r, rgb.g, rgb.b)[2]),
      vibrant: hslToHex(h, Math.min(s, 0.5), 0.35),
    };
  } catch {
    return null;
  }
}

const compactFormatter = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

export interface BlueprintCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  visibility?: string;
  avatarUrl?: string;
  variant?: "default" | "oftenUsedTogether" | "list";
  deployCount?: number;
  heartCount?: number;
  isDraft?: boolean;
  /** When provided, shows a three-dot menu with an archive option. */
  onArchive?: () => void;
}

export function BlueprintCard({
  slug,
  account,
  name,
  description,
  visibility,
  avatarUrl,
  variant = "default",
  deployCount,
  heartCount,
  isDraft = false,
  onArchive,
}: BlueprintCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const formattedDeploys = deployCount != null ? compactFormatter.format(deployCount) : "0";
  const deployLabel = deployCount === 1 ? "deploy" : "deploys";
  const [accent, setAccent] = useState<CardAccent | null>(null);
  const handleAvatarLoad = useCallback((e: React.SyntheticEvent<HTMLImageElement>) => {
    const colors = extractAccentFromImg(e.currentTarget);
    if (colors) setAccent(colors);
  }, []);

  if (variant === "oftenUsedTogether") {
    return (
      <Link
        to={`/${slug}`}
        className="group flex items-center gap-3 overflow-hidden rounded-md border border-border-strong bg-stone-100 px-3 py-2 transition-all duration-150 hover:bg-stone-200 hover:border-teal-500 hover:shadow-md dark:bg-muted/30 dark:hover:border-teal-400"
      >
        <BlueprintIdentity
          account={account}
          name={name}
          size={36}
          className="size-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="truncate text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
            {name}
          </h3>
          <p className="flex items-center gap-1.5 font-mono text-mono-sm text-faint-foreground">
            {formattedDeploys} {deployLabel}
            <span className="text-border-strong">•</span>
            {account}
          </p>
        </div>
      </Link>
    );
  }

  if (variant === "list") {
    const formattedHearts = heartCount != null ? compactFormatter.format(heartCount) : "0";
    return (
      <>
        <div className="flex items-center gap-4 rounded-lg border border-border bg-background px-4 py-3">
          <Link to={`/${slug}`} className="flex min-w-0 flex-1 items-center gap-3">
            <BlueprintIdentity
              account={account}
              name={name}
              size={36}
              url={avatarUrl}
              className="size-9 shrink-0 overflow-hidden rounded-sm"
            />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate text-heading-4 text-foreground">{name}</span>
                {isDraft
                  ? <StatusBadge color="warning">Finish setup</StatusBadge>
                  : visibility === "private" && <PrivacyBadge />
                }
              </div>
              {description && (
                <p className="truncate text-body-sm text-muted-foreground">{description}</p>
              )}
            </div>
          </Link>
          <div className="flex shrink-0 items-center gap-2">
            {isDraft ? (
              <Button asChild size="sm" variant="outline">
                <Link to={`/${slug}`}>
                  Continue setup
                  <ArrowRightIcon className="size-3.5" />
                </Link>
              </Button>
            ) : (
              <>
                <span className="font-mono text-mono-sm text-muted-foreground">
                  {formattedDeploys} deploys · {formattedHearts} hearts
                </span>
                <Button asChild size="sm">
                  <Link to={`/deploy/${slug}`}>Deploy</Link>
                </Button>
                <Button asChild size="sm" variant="outline">
                  <Link to={`/${slug}`}>View →</Link>
                </Button>
              </>
            )}
            {onArchive && (
              <div onClick={(e) => { e.preventDefault(); e.stopPropagation(); }}>
                <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      className="flex h-7 w-7 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                      aria-label="Blueprint options"
                    >
                      <EllipsisHorizontalIcon className="h-4 w-4" />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="rounded-[10px] p-0">
                    <DropdownMenuItem
                      variant="destructive"
                      onSelect={() => { setMenuOpen(false); setArchiveOpen(true); }}
                      className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
                    >
                      <ArchiveBoxIcon className="h-4 w-4" />
                      Archive
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )}
          </div>
        </div>
        {onArchive && (
          <ArchiveBlueprintDialog
            open={archiveOpen}
            onOpenChange={setArchiveOpen}
            blueprintName={name}
            account={account}
            onArchived={onArchive}
          />
        )}
      </>
    );
  }

  const cardHref = `/${slug}`;

  // Derive stable fold lines from the slug — 1 or 2 folds, non-overlapping
  const folds = useMemo(() => {
    // Simple seeded PRNG from the slug
    let s = 1;
    for (let i = 0; i < slug.length; i++) s = ((s << 5) - s + slug.charCodeAt(i)) | 0;
    s = Math.abs(s) || 1;
    const rand = () => { s = (s * 16807) % 2147483647; return s / 2147483647; };

    const count = rand() > 0.5 ? 2 : 1;
    const result: { position: number; rotation: number }[] = [];

    for (let i = 0; i < count; i++) {
      rand(); // consume to keep sequence stable
      const rotation = rand() * 10 - 5; // -5 to 5 degrees

      // Place folds in non-overlapping zones: split the card into thirds
      // First fold goes in the first 2/3, second fold in the last 2/3
      let position: number;
      if (count === 1) {
        position = 25 + rand() * 50; // 25%-75%
      } else {
        position = i === 0
          ? 20 + rand() * 25  // 20%-45%
          : 55 + rand() * 25; // 55%-80%
      }

      result.push({ position, rotation });
    }
    return result;
  }, [slug]);

  return (
    <>
      <Link
        to={cardHref}
        className={cn(
          "group relative flex flex-col overflow-hidden shadow-sm transition-all duration-150 hover:shadow-md",
          isDraft
            ? "border-[6px] border-dashed border-stone-400 dark:border-teal-800 bg-transparent"
            : "[--mix:18%] hover:[--mix:14%] border-[0.5px] border-white transition-[background-color] duration-150 dark:bg-teal-900/30 before:pointer-events-none before:absolute before:inset-0 before:z-0 before:bg-[length:8px_8px] before:bg-[linear-gradient(to_right,rgb(255_255_255/0.5)_0.5px,transparent_0.5px),linear-gradient(to_bottom,rgb(255_255_255/0.5)_0.5px,transparent_0.5px)] after:pointer-events-none after:absolute after:inset-[3px] after:border-2 after:border-white dark:after:border-teal-800"
        )}
        style={accent && !isDraft ? {
          backgroundColor: `color-mix(in srgb, ${accent.base} var(--mix), white)`,
          '--card-accent': accent.vibrant,
        } as React.CSSProperties : undefined}
      >
        {!isDraft && folds.map((fold, i) => {
          const grad = accent
            ? `linear-gradient(to right, transparent, color-mix(in srgb, ${accent.base} 3.4%, transparent) 45%, color-mix(in srgb, ${accent.base} 6%, transparent) 50%, transparent 50.5%, transparent)`
            : "linear-gradient(to right, transparent, rgb(0 0 0 / 0.025) 45%, rgb(0 0 0 / 0.05) 50%, transparent 50.5%, transparent)";
          return (
            <div
              key={i}
              className="pointer-events-none absolute z-0"
              style={{
                top: 0, bottom: 0,
                left: `${fold.position - 25}%`,
                width: "50%",
                background: grad,
                transform: `rotate(${fold.rotation}deg)`,
              }}
            />
          );
        })}
        {!isDraft && (
          <div
            className="pointer-events-none absolute inset-0 z-0"
            style={{ background: "linear-gradient(120deg, transparent 0%, rgb(255 255 255 / 0.75) 100%)" }}
          />
        )}
        {onArchive && (
          <div
            className="absolute top-3 right-3"
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); }}
          >
            <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  className="flex h-7 w-7 items-center justify-center rounded-sm text-foreground transition-colors hover:bg-accent"
                  aria-label="Blueprint options"
                >
                  <EllipsisHorizontalIcon className="h-4 w-4" />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="rounded-[10px] p-0">
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => {
                    setMenuOpen(false);
                    setArchiveOpen(true);
                  }}
                  className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
                >
                  <ArchiveBoxIcon className="h-4 w-4" />
                  Archive
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )}
        <div className="relative z-[1] flex flex-1 items-start gap-3 p-4 pb-3" style={{ textShadow: "0 0 5px rgb(255 255 255 / 0.8), 0 0 10px rgb(255 255 255 / 0.8), 0 0 16px rgb(255 255 255 / 0.8)" }}>
          <BlueprintIdentity
            account={account}
            name={name}
            size={36}
            url={avatarUrl}
            className="size-9 shrink-0 overflow-hidden border-[0.5px] border-white rounded-[3px]"
            onLoad={handleAvatarLoad}
          />
          <div className={cn("flex min-w-0 flex-1 flex-col gap-1", onArchive ? "pr-8" : "pr-1")}>
            <h3 className={cn(
              "flex min-w-0 items-center gap-1.5 text-heading-4 text-foreground transition-colors",
              accent ? "group-hover:[color:var(--card-accent)]" : "group-hover:text-teal-500 dark:group-hover:text-teal-400"
            )}>
              <span className="truncate">{name}</span>
              {isDraft
                ? <StatusBadge color="warning">Finish setup</StatusBadge>
                : visibility === "private" && <PrivacyBadge onClick={(e) => e.preventDefault()} />
              }
            </h3>
            <p className="line-clamp-3 text-body-sm text-foreground/70">
              {description}
            </p>
          </div>
        </div>
        {!isDraft && <div className="relative z-[1] mx-[5px] h-px bg-white" />}
        <div
          className={cn("relative z-[1] flex items-center justify-between px-4 py-2.5", isDraft ? "border-t border-dashed border-border" : "pb-3.5")}
          style={accent && !isDraft ? { color: `color-mix(in srgb, ${accent.base} 70%, black)` } : undefined}
        >
          <span className={cn("text-mono-sm font-mono", accent && !isDraft ? "text-[inherit]" : "text-faint-foreground")}>
            {formattedDeploys} {deployLabel}
          </span>
          <span className={cn("flex items-center gap-1.5 text-mono-sm font-mono", accent && !isDraft ? "text-[inherit]" : "text-faint-foreground")}>
            <UserAvatar handle={account} name={account} className="!size-4" />
            {account}
          </span>
        </div>
      </Link>

      {onArchive && (
        <ArchiveBlueprintDialog
          open={archiveOpen}
          onOpenChange={setArchiveOpen}
          blueprintName={name}
          account={account}
          onArchived={onArchive}
        />
      )}
    </>
  );
}
