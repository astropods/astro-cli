import { useState, useMemo } from "react";
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
import type { AvatarColors } from "@/lib/api";

interface CardAccent {
  base: string;
  vibrant: string;
  vibrantLight: string;
}

// Derived from the Astro teal accent (#14b8a6) using the same formula as
// colorextract.ExtractFromRGBA, matching DEFAULT_COLORS in astro-trading-card.
const DEFAULT_ACCENT: CardAccent = {
  base: "#3d8f86",
  vibrant: "#2d867c",
  vibrantLight: "#85e0d6",
};

/** sRGB relative luminance (0–1) from a hex color — perceptual brightness. */
function srgbLuminance(hex: string): number {
  const r = parseInt(hex.slice(1, 3), 16) / 255;
  const g = parseInt(hex.slice(3, 5), 16) / 255;
  const b = parseInt(hex.slice(5, 7), 16) / 255;
  const lin = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

/** Light-mode accent adjustments scaled by perceptual brightness.
 *  Brighter accents get more oklch darkening and higher mix; darker ones pass through. */
function lightModeAccent(accentHex: string): { mix: number; mixHover: number; strength: number } {
  const lum = srgbLuminance(accentHex);
  const t = Math.min(1, Math.max(0, lum / 0.5));
  const mix = 26 + t * 16; // 26% (dark accent) → 42% (light accent)
  const strength = 100 - t * 20; // 100% (dark, no change) → 80% (light, 20% black in oklch)
  return { mix, mixHover: mix * 0.8, strength };
}

/** Deterministic fold lines derived from a slug string. */
function generateFoldLines(slug: string): { position: number; rotation: number }[] {
  // Simple seeded PRNG (Park-Miller LCG)
  let s = 1;
  for (let i = 0; i < slug.length; i++) s = ((s << 5) - s + slug.charCodeAt(i)) | 0;
  s = Math.abs(s) || 1;
  const rand = () => { s = (s * 16807) % 2147483647; return s / 2147483647; };

  const count = rand() > 0.5 ? 2 : 1;
  const result: { position: number; rotation: number }[] = [];

  for (let i = 0; i < count; i++) {
    rand(); // consume to keep sequence stable
    const rotation = rand() * 10 - 5; // -5 to 5 degrees

    // Place folds in non-overlapping zones
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
}

// className fragments shared by both draft and non-draft card treatments
const cardVars = "[--card-neutral:oklch(97.76%_0.0106_194.137)] dark:[--card-neutral:#0a1614] [--card-contrast:black] dark:[--card-contrast:white] [--card-grid:rgb(255_255_255/0.5)] dark:[--card-grid:rgb(255_255_255/0.07)]";
const cardAccentVars = "[--mix:var(--mix-base)] hover:[--mix:var(--mix-hover)] dark:[--mix:18%] dark:hover:[--mix:14%]";
const cardBorder = "border-[0.5px] border-teal-25 dark:border-white/10";
const gridOverlay = "before:pointer-events-none before:absolute before:inset-0 before:z-0 before:bg-[length:8px_8px] before:bg-[linear-gradient(to_right,var(--card-grid)_0.5px,transparent_0.5px),linear-gradient(to_bottom,var(--card-grid)_0.5px,transparent_0.5px)]";
const innerBorderBase = "after:pointer-events-none after:absolute after:inset-[3px] after:border-2 after:border-teal-25 dark:after:border-white/10";
const innerBorderDashed = "after:pointer-events-none after:absolute after:inset-0 after:border after:border-dashed after:border-stone-300 dark:after:border-white/10";
const draftBorder = "";
const draftVars = "[--card-neutral:rgb(255_255_255/0.25)] dark:[--card-neutral:rgb(0_0_0/0.25)]";
const fadeOverlayStyle = { background: "linear-gradient(120deg, transparent 0%, color-mix(in srgb, var(--card-neutral) 75%, transparent) 100%)" } as const;
const contentShadowStyle = { textShadow: "0 0 5px color-mix(in srgb, var(--card-neutral) 80%, transparent), 0 0 10px color-mix(in srgb, var(--card-neutral) 80%, transparent), 0 0 16px color-mix(in srgb, var(--card-neutral) 80%, transparent)" } as const;

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
  avatarColors?: AvatarColors;
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
  avatarColors,
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
  const accent: CardAccent = avatarColors ? {
    base: avatarColors.base,
    vibrant: avatarColors.vibrant,
    vibrantLight: avatarColors.vibrant_light,
  } : DEFAULT_ACCENT;
  const hasAccent = !isDraft;

  const folds = useMemo(() => generateFoldLines(slug), [slug]);

  const foldGradient = hasAccent
    ? `linear-gradient(to right, transparent, color-mix(in srgb, ${accent.base} 3.4%, transparent) 45%, color-mix(in srgb, ${accent.base} 6%, transparent) 50%, transparent 50.5%, transparent)`
    : "linear-gradient(to right, transparent, rgb(0 0 0 / 0.025) 45%, rgb(0 0 0 / 0.05) 50%, transparent 50.5%, transparent)";

  const cardShell = cn(
    "relative overflow-hidden bg-[var(--card-neutral)]",
    hasAccent
      ? cn(cardVars, cardBorder, cardAccentVars, "transition-[background-color] duration-150", gridOverlay, innerBorderBase)
      : cn(draftVars, draftBorder, innerBorderDashed),
  );

  const accentAdj = useMemo(() => lightModeAccent(accent.base), [accent.base]);
  const darkenedAccent = `color-mix(in oklch, ${accent.base} ${accentAdj.strength}%, black)`;

  const cardStyle = hasAccent ? {
    backgroundColor: `color-mix(in srgb, ${darkenedAccent} var(--mix), var(--card-neutral))`,
    '--card-accent': accent.vibrant,
    '--card-accent-light': accent.vibrantLight,
    '--card-muted': `color-mix(in srgb, ${darkenedAccent} 70%, var(--card-contrast))`,
    '--mix-base': `${accentAdj.mix}%`,
    '--mix-hover': `${accentAdj.mixHover}%`,
  } as React.CSSProperties : undefined;

  const cardOverlays = (
    <>
      {hasAccent && folds.map((fold, i) => (
        <div
          key={i}
          className="pointer-events-none absolute inset-y-0 z-0 w-1/2"
          style={{
            left: `${fold.position - 25}%`,
            background: foldGradient,
            transform: `rotate(${fold.rotation}deg)`,
          }}
        />
      ))}
      <div
        className="pointer-events-none absolute inset-0 z-0"
        style={fadeOverlayStyle}
      />
    </>
  );

  if (variant === "oftenUsedTogether") {
    return (
      <Link
        to={`/${slug}`}
        className={cn(cardShell, "group flex items-center gap-3 pl-2 pr-3 py-2 shadow-sm transition-all duration-150 hover:shadow-md after:border")}
        style={cardStyle}
      >
        {cardOverlays}
        <BlueprintIdentity
          account={account}
          name={name}
          size={36}
          url={avatarUrl}
          className="relative z-[1] size-9 shrink-0 overflow-hidden border-[0.5px] border-teal-25 dark:border-white/20 rounded-[3px]"
        />
        <div className="relative z-[1] flex min-w-0 flex-1 flex-col gap-1" style={contentShadowStyle}>
          <h3 className={cn(
            "truncate text-heading-4 text-foreground transition-colors",
            hasAccent ? "group-hover:[color:var(--card-accent)] dark:group-hover:[color:var(--card-accent-light)]" : "group-hover:text-teal-500 dark:group-hover:text-teal-400"
          )}>
            {name}
          </h3>
          <p className={cn("flex items-center gap-1.5 font-mono text-mono-sm", hasAccent ? "text-[var(--card-muted)]" : "text-faint-foreground")}>
            {formattedDeploys} {deployLabel}
            <span className="opacity-40">•</span>
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
        <div
          className={cn(cardShell, !isDraft && "shadow-sm")}
          style={cardStyle}
        >
          {cardOverlays}
          <div className="relative z-[1] flex items-center gap-4 pl-2 pr-4 py-3" style={contentShadowStyle}>
            <Link to={`/${slug}`} className="flex min-w-0 flex-1 items-center gap-3">
              <BlueprintIdentity
                account={account}
                name={name}
                size={36}
                url={avatarUrl}
                className="relative z-[1] size-9 shrink-0 overflow-hidden border-[0.5px] border-teal-25 dark:border-white/20 rounded-[3px]"
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
                  <p className="truncate text-body-sm text-foreground/70">{description}</p>
                )}
              </div>
            </Link>
            <div className="flex shrink-0 items-center gap-2" style={{ textShadow: "none" }}>
              {isDraft ? (
                <Button asChild size="sm" variant="outline">
                  <Link to={`/${slug}`}>
                    Continue setup
                    <ArrowRightIcon className="size-3.5" />
                  </Link>
                </Button>
              ) : (
                <>
                  <span className={cn("font-mono text-mono-sm", hasAccent ? "text-[var(--card-muted)]" : "text-faint-foreground")}>
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

  return (
    <>
      <Link
        to={cardHref}
        className={cn(cardShell, "group flex flex-col transition-all duration-150", !isDraft && "shadow-sm hover:shadow-md")}
        style={cardStyle}
      >
        {cardOverlays}
        {onArchive && (
          <div
            className="absolute top-3 right-3 z-[2]"
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
        <div className="relative z-[1] flex flex-1 items-start gap-3 p-4 pb-3" style={contentShadowStyle}>
          <BlueprintIdentity
            account={account}
            name={name}
            size={36}
            url={avatarUrl}
            className="size-9 shrink-0 overflow-hidden border-[0.5px] border-teal-25 dark:border-white/20 rounded-[3px]"
          />
          <div className={cn("flex min-w-0 flex-1 flex-col gap-1", onArchive ? "pr-8" : "pr-1")}>
            <h3 className={cn(
              "flex min-w-0 items-center gap-1.5 text-heading-4 text-foreground transition-colors",
              hasAccent ? "group-hover:[color:var(--card-accent)] dark:group-hover:[color:var(--card-accent-light)]" : "group-hover:text-teal-500 dark:group-hover:text-teal-400"
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
        <div className={cn("relative z-[1] mx-[5px] border-t dark:border-white/10", isDraft ? "border-dashed border-stone-300" : "border-teal-25")} />
        <div
          className={cn("relative z-[1] flex items-center justify-between px-4 py-2.5 pb-3.5")}
          style={hasAccent ? { color: `color-mix(in srgb, ${darkenedAccent} 70%, var(--card-contrast))` } : undefined}
        >
          <span className={cn("text-mono-sm font-mono", hasAccent ? "text-[inherit]" : "text-faint-foreground")}>
            {formattedDeploys} {deployLabel}
          </span>
          <span className={cn("flex items-center gap-1.5 text-mono-sm font-mono", hasAccent ? "text-[inherit]" : "text-faint-foreground")}>
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
