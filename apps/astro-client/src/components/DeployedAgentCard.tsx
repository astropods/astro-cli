import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router";
import { RequestSparkline, ZERO_SERIES } from "@/components/RequestSparkline";
import {
  ArrowUpRightIcon,
  ArrowPathIcon,
  BookOpenIcon,
  CheckIcon,
  Cog6ToothIcon,
  DocumentDuplicateIcon,
  EllipsisHorizontalIcon,
  ShareIcon,
} from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { AvatarImage } from "@/components/AvatarImage";
import { getAgentAvatarUrl } from "@/lib/assets";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { StatusBadge } from "@/components/StatusBadge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { TradingCardModal } from "@/components/trading-card/TradingCardModal";
import { useBlueprint } from "@/api/queries/blueprints";
import { getBlueprintIntegrations } from "@/lib/blueprint-utils";
import { formatDate, getLaunchDisabledMessage } from "@/lib/deployment-utils";
import type { CardData } from "astro-trading-card";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { chatDeploymentPath, deploymentConfigurePath, deploymentPath } from "@/lib/routes";
import type { AvatarColors } from "@/lib/api";
import cloud1 from "@/assets/clouds/cloud-1.png";
import cloud2 from "@/assets/clouds/cloud-2.png";
import cloud3 from "@/assets/clouds/cloud-3.png";
import cloud4 from "@/assets/clouds/cloud-4.png";

const CLOUD_SPRITES = [cloud1, cloud2, cloud3, cloud4];

const CARD_INTERACTIVE_SELECTOR = [
  "a[href]",
  "button",
  "input",
  "select",
  "textarea",
  "[role='button']",
  "[role='link']",
  "[role='menuitem']",
  "[tabindex]:not([tabindex='-1'])",
  "[data-card-click-ignore='true']",
].join(",");

export interface DeployedAgentCardProps {
  account: string;
  name: string;
  displayName?: string;
  /** Stable identifier used to seed the per-card starfield. Falls back to
   *  `account/name` when omitted. */
  deploymentId?: string;
  avatarUrl?: string;
  avatarColors?: AvatarColors;
  /** Daily request counts, oldest → newest. Typically 7 values. */
  requestSeries?: number[];
  /** Daily token totals, oldest → newest. Normalized independently from
   *  requestSeries so the two lines overlay cleanly. */
  tokenSeries?: number[];
  /** Whether to surface the "Launch" CTA (opens the in-app chat for this
   *  deployment). When false/omitted the action row collapses to a single
   *  full-width "Manage agent" button. Requires `deploymentId` to navigate. */
  canLaunch?: boolean;
  /** Whether the Launch button should be disabled (e.g., during deployment). */
  launchDisabled?: boolean;
  /** Current deployment status for tooltip messaging. */
  deploymentStatus?: string;
  /** Surfaces an error pill under the subline. */
  hasError?: boolean;
  /** Deployment creation timestamp; surfaced in the badge modal's stats row. */
  installedAt?: string;
  /** Surfaces an "Update available" pill under the subline. */
  hasUpdateAvailable?: boolean;
  /** Latest published build id for this agent's lineage. When provided
   *  together with `hasUpdateAvailable`, the pill becomes a link to the
   *  configure form pre-loaded with that build — same flow as the
   *  detail page's update affordance. */
  latestBuildId?: string;
  /** Creator-only receipt state while WorkOS registers the resource and role. */
  accessProvisioning?: boolean;
  /** True after access setup has exceeded the normal short convergence window. */
  accessProvisioningDelayed?: boolean;
  /** True after access setup exceeds the bounded automatic retry window. */
  accessProvisioningStalled?: boolean;
  onRetryAccess?: () => void;
  className?: string;
}

function eventStartedFromCardInteractive(target: EventTarget | null, currentTarget: HTMLElement) {
  if (!(target instanceof Element)) return false;
  const interactiveElement = target.closest(CARD_INTERACTIVE_SELECTOR);
  return !!interactiveElement && interactiveElement !== currentTarget && currentTarget.contains(interactiveElement);
}

// 32-bit FNV-1a-ish string hash for seeding mulberry32. Same shape as the
// hash used by BlueprintCard's fold-line generator — different bytes in,
// different stars out, but a stable mapping per agent.
function hashSeed(s: string): number {
  let h = 1;
  for (let i = 0; i < s.length; i++) h = ((h << 5) - h + s.charCodeAt(i)) | 0;
  return Math.abs(h) || 1;
}

// Mulberry32: tiny deterministic PRNG. Lifted from the canvas StarField so the
// card's static field uses the same distribution shape (just no animation).
function mulberry32(seed: number) {
  return () => {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const STAR_COUNT = 150;
// Same sprite constants the canvas StarField uses (see star-sprite.ts):
// drawSize = baseR / SPRITE_DRAW_SCALE, with a solid inner core out to
// SPRITE_BRIGHT_STOP that hard-cuts to transparent. Mirrored as an SVG
// radial gradient so card stars match the page starfield's shape.
const STAR_DRAW_SCALE = 0.11;
const STAR_BRIGHT_STOP = 0.07;
const STAR_CORE_ALPHA = 0.55;
// Logical radius range — bigger than the canvas SIZE_RANGE since the card's
// viewport is smaller; stars need more presence at this scale.
const STAR_BASE_R: [number, number] = [1.2, 2.8];

// Asymmetric ramp: snappy spool-up on hover-in, languid coast-down on hover-out.
const RAMP_UP_MS = 150;
const RAMP_DOWN_MS = 1000;
const CARD_TITLE_MAX_CHARS = 22;

function getCardTitleDisplay(title: string): string {
  const chars = Array.from(title.trim());
  if (chars.length <= CARD_TITLE_MAX_CHARS) return title;
  return `${chars.slice(0, CARD_TITLE_MAX_CHARS).join("").trimEnd()}\u2026`;
}

function CardStarfield({ seed, hovered }: { seed: string; hovered: boolean }) {
  const starGradientId = useId();
  const cloudMaskId = useId();
  const seedNum = hashSeed(seed);
  const svgRef = useRef<SVGSVGElement>(null);
  const animationGroupsRef = useRef<{
    all: Animation[];
    drift: Animation[];
    twinkle: Animation[];
  } | null>(null);
  // Ramp playbackRate 0↔1 on hover via RAF. Drift ramps in both directions for
  // the smooth spool-up / coast-down feel. Twinkles ramp UP with drift, but on
  // hover-out they snap to 0 immediately — slow-fading a mid-flash sparkle
  // makes it linger weirdly.
  useEffect(() => {
    // Leave untouched cards in their CSS-paused state. Activating every star
    // animation during hydration is expensive even at playbackRate=0.
    if (!hovered && !animationGroupsRef.current) return;

    const svg = svgRef.current;
    if (!svg || typeof svg.getAnimations !== "function") return;
    let animationGroups = animationGroupsRef.current;
    if (!animationGroups) {
      const all = svg.getAnimations({ subtree: true });
      if (all.length === 0) return;
      const isTwinkle = (animation: Animation) =>
        (animation as CSSAnimation).animationName === "card-star-twinkle";
      animationGroups = {
        all,
        drift: all.filter((animation) => !isTwinkle(animation)),
        twinkle: all.filter(isTwinkle),
      };
      animationGroupsRef.current = animationGroups;
    }
    const { all: animations, drift: driftAnims, twinkle: twinkleAnims } = animationGroups;
    // CSS births these in animation-play-state:paused so they don't tick at
    // full speed on first paint. On first hover, flip this card's animations
    // to running at rate 0 so the ramp below can drive playbackRate smoothly.
    for (const a of animations) {
      if (a.playState === "paused") {
        a.playbackRate = 0;
        a.play();
      }
    }
    const target = hovered ? 1 : 0;
    if (!hovered) {
      for (const a of twinkleAnims) a.playbackRate = 0;
    }
    const driftStart = driftAnims[0]?.playbackRate ?? 0;
    const twinkleStart = twinkleAnims[0]?.playbackRate ?? 0;
    if (driftStart === target && (hovered ? twinkleStart === target : true)) {
      if (!hovered) {
        for (const a of animations) a.pause();
      }
      return;
    }
    const duration = hovered ? RAMP_UP_MS : RAMP_DOWN_MS;
    const startTime = performance.now();
    let rafId = 0;
    const tick = (now: number) => {
      const t = Math.min(1, (now - startTime) / duration);
      // ease-in-out cubic
      const eased = t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
      const driftRate = driftStart + (target - driftStart) * eased;
      for (const a of driftAnims) a.playbackRate = driftRate;
      if (hovered) {
        const twinkleRate = twinkleStart + (target - twinkleStart) * eased;
        for (const a of twinkleAnims) a.playbackRate = twinkleRate;
      }
      if (t < 1) {
        rafId = requestAnimationFrame(tick);
      } else if (!hovered) {
        // Preserve the final frame without leaving non-hovered cards in the
        // browser's running animation registry.
        for (const a of animations) a.pause();
      }
    };
    rafId = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafId);
  }, [hovered]);
  const stars = useMemo(() => {
    const rand = mulberry32(seedNum);
    return Array.from({ length: STAR_COUNT }, () => {
      // sizeFactor doubles as a depth proxy: 0 = far/small, 1 = close/big.
      const sizeFactor = rand();
      const r = (STAR_BASE_R[0] + sizeFactor * (STAR_BASE_R[1] - STAR_BASE_R[0])) / (STAR_DRAW_SCALE * 2);
      // Closer stars move faster (shorter duration) → parallax.
      const duration = 32 - sizeFactor * 16; // far ≈ 32s, close ≈ 16s
      // Negative delay staggers the start so stars don't all snap together
      // at the wrap boundary.
      const delay = -rand() * duration;
      // ~12% of stars get a flash animation; long per-star duration means
      // any single star sparkles infrequently and only a few do at once.
      const twinkles = rand() < 0.12;
      const twinkleDuration = twinkles ? 10 + rand() * 14 : 0;
      const twinkleDelay = twinkles ? -rand() * twinkleDuration : 0;
      return {
        cx: rand() * 100,
        cy: rand() * 100,
        r,
        o: 0.25 + rand() * 0.6,
        duration,
        delay,
        twinkles,
        twinkleDuration,
        twinkleDelay,
      };
    });
  }, [seedNum]);
  // Independent seeded RNG for cloud placement so star tweaks don't shift the
  // cloud. The sprite is used as an alpha mask over a flat `var(--card-cloud)`
  // fill, matching the canvas StarField's recolor-via-tint behavior.
  const cloud = useMemo(() => {
    const rand = mulberry32(seedNum * 7 + 13);
    return {
      src: CLOUD_SPRITES[Math.floor(rand() * CLOUD_SPRITES.length)],
      cx: 20 + rand() * 60,
      cy: 12 + rand() * 28,
      size: 150 + rand() * 80, // 150-230% of card width — sprite has heavy feather
      rotation: rand() * 360,
      opacity: 0.02 + rand() * 0.04, // 0.02-0.06 — barely-there wash
    };
  }, [seedNum]);
  return (
    <svg
      ref={svgRef}
      className="pointer-events-none absolute inset-0 h-full w-full"
      style={{
        color: "var(--card-contrast)",
        maskImage: "linear-gradient(to bottom, black 50%, transparent 95%)",
        WebkitMaskImage: "linear-gradient(to bottom, black 50%, transparent 95%)",
      }}
      aria-hidden="true"
    >
      <defs>
        <radialGradient id={starGradientId} cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="currentColor" stopOpacity={STAR_CORE_ALPHA} />
          <stop offset={`${STAR_BRIGHT_STOP * 100}%`} stopColor="currentColor" stopOpacity={STAR_CORE_ALPHA} />
          <stop offset={`${(STAR_BRIGHT_STOP + 0.01) * 100}%`} stopColor="currentColor" stopOpacity="0" />
          <stop offset="100%" stopColor="currentColor" stopOpacity="0" />
        </radialGradient>
        {/* mask-type:alpha so the sprite's RGB (always white) is ignored —
            only its alpha channel drives visibility. Set via style because
            React's SVG types omit `maskType`. */}
        <mask id={cloudMaskId} style={{ maskType: "alpha" }}>
          <image
            href={cloud.src}
            x={`${cloud.cx - cloud.size / 2}%`}
            y={`${cloud.cy - cloud.size / 2}%`}
            width={`${cloud.size}%`}
            height={`${cloud.size}%`}
            transform={`rotate(${cloud.rotation} ${cloud.cx} ${cloud.cy})`}
            style={{ transformOrigin: `${cloud.cx}% ${cloud.cy}%` }}
            preserveAspectRatio="xMidYMid meet"
          />
        </mask>
      </defs>
      <rect
        x="0"
        y="0"
        width="100%"
        height="100%"
        fill="var(--card-cloud)"
        mask={`url(#${cloudMaskId})`}
        opacity={cloud.opacity}
        style={{ mixBlendMode: "screen" }}
      />
      {stars.map((s, i) => (
        // Two-element wrapper: outer <g> owns the drift transform, inner
        // <circle> owns the twinkle scale. Keeps the two transforms on
        // separate elements so they don't fight each other.
        <g
          key={i}
          className="card-v2-star-anim"
          style={{
            animationName: "card-star-drift",
            animationDuration: `${s.duration}s`,
            animationDelay: `${s.delay}s`,
            animationTimingFunction: "linear",
            animationIterationCount: "infinite",
          }}
        >
          <circle
            cx={`${s.cx}%`}
            cy={`${s.cy}%`}
            r={s.r}
            fill={`url(#${starGradientId})`}
            className="card-v2-star"
            style={{ ["--star-o" as string]: s.o }}
          />
          {/* Flash overlay — invisible most of the time, jumps to opaque white
              for a single peak per twinkle cycle. Sized to the star's bright
              core so the flash reads as the star itself "popping" rather than
              a new dot appearing. */}
          {s.twinkles && (
            <circle
              cx={`${s.cx}%`}
              cy={`${s.cy}%`}
              r={s.r * 0.12}
              fill="white"
              opacity={0}
              className="card-v2-star-anim"
              style={{
                animationName: "card-star-twinkle",
                animationDuration: `${s.twinkleDuration}s`,
                animationDelay: `${s.twinkleDelay}s`,
                animationTimingFunction: "linear",
                animationIterationCount: "infinite",
              }}
            />
          )}
        </g>
      ))}
    </svg>
  );
}

export function DeployedAgentCard({
  account,
  name,
  displayName,
  deploymentId,
  avatarUrl,
  avatarColors,
  requestSeries,
  tokenSeries,
  canLaunch,
  launchDisabled = false,
  deploymentStatus,
  hasError,
  installedAt,
  hasUpdateAvailable,
  latestBuildId,
  accessProvisioning = false,
  accessProvisioningDelayed = false,
  accessProvisioningStalled = false,
  onRetryAccess,
  className,
}: DeployedAgentCardProps) {
  // Vertical fade: avatar tint at the top of the card, fading to the bare
  // card surface at the bottom so the buttons sit on neutral chrome.
  // --card-base is a theme-aware override: in dark mode it drops one token
  // (slate-800 → slate-900) so the card sits closer to the page background.
  const tintStyle: React.CSSProperties | undefined = avatarColors
    ? {
        background: `linear-gradient(to bottom,
          color-mix(in srgb, ${avatarColors.base} var(--tint-strong, 22%), var(--card-base)) 0%,
          var(--card-base) 60%)`,
        // Cloud picks up the agent's own glow color (or vibrant_light fallback).
        // Painted onto the cloud rect with mix-blend-mode: screen so it lightens
        // the underlying gradient instead of overwriting it.
        ["--card-cloud" as string]: avatarColors.glow ?? avatarColors.vibrant_light,
      }
    : undefined;
  const starSeed = deploymentId ?? `${account}/${name}`;
  const [hovered, setHovered] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [shareOpen, setShareOpen] = useState(false);
  const { copy: copyToClipboard, copied } = useCopyToClipboard(1600);
  const navigate = useNavigate();
  // Fetch blueprint only when the badge modal is opened — keeps the dashboard
  // grid from paying for N blueprint queries on first paint.
  const { data: blueprint } = useBlueprint(account, name, { enabled: shareOpen });
  const integrations = blueprint ? getBlueprintIntegrations(blueprint) : [];
  const cardData = useMemo<CardData>(() => {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return {
      name,
      displayName,
      account,
      avatar: avatarUrl ? { url: avatarUrl } : undefined,
      stats: [
        ...(installedAt
          ? [{ label: "Deployed", value: formatDate(installedAt) }]
          : []),
        { label: "From", value: `${account}/${name}`, wrap: true },
      ],
      barcodeId: deploymentId,
      qrUrl: `${origin}/${account}/${name}`,
    };
  }, [name, displayName, account, avatarUrl, installedAt, deploymentId]);
  // Card-level clicks and the Manage action share the builder-focused default:
  // the deployment history and controls page.
  const detailPath = deploymentId && !accessProvisioning
    ? deploymentPath(account, deploymentId)
    : undefined;
  const copyId = () => {
    if (deploymentId) void copyToClipboard(deploymentId);
  };
  const handleCardClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!detailPath || eventStartedFromCardInteractive(e.target, e.currentTarget)) return;
    navigate(detailPath);
  };
  const handleCardKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (!detailPath || e.target !== e.currentTarget) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      navigate(detailPath);
    }
  };
  // Menu visibility: revealed on card hover OR while the menu (or the share
  // modal) is open — otherwise dismissing the menu by mouse-leaving the card
  // while the dropdown was still open would yank the trigger out from under
  // the user.
  const menuVisible = hovered || menuOpen || shareOpen;
  const agentTitle = displayName || name;
  const agentTitleDisplay = getCardTitleDisplay(agentTitle);
  return (
    <div
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={handleCardClick}
      onKeyDown={handleCardKeyDown}
      data-deployment-id={deploymentId}
      role={detailPath ? "link" : undefined}
      tabIndex={detailPath ? 0 : undefined}
      aria-label={detailPath ? `View details for ${agentTitle}` : undefined}
      className={cn(
        "relative flex flex-col items-center gap-4 overflow-hidden rounded-md border border-border bg-[var(--card-base)] p-4 pt-8",
        // --card-base is the card's underlying surface. Default to var(--card);
        // in dark mode step one token deeper (--surface) so the card reads as
        // a quiet panel on the page background rather than a lifted chrome.
        "[--card-base:var(--card)] dark:[--card-base:var(--surface)]",
        // Stars stay white in both modes. In light mode they barely show
        // against the bg gradient — that's intentional, a quiet field.
        "[--card-contrast:white]",
        // --card-cloud has a neutral fallback for cards without avatarColors;
        // when avatarColors is set, inline style overrides this with the
        // agent's `glow` color so each card's nebula carries its own theme.
        "[--card-cloud:#ffffff] dark:[--card-cloud:oklch(58.40%_0.2055_274.722)]",
        avatarColors && "[--tint-strong:30%] dark:[--tint-strong:18%]",
        detailPath && "cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
      style={tintStyle}
    >
      <CardStarfield seed={starSeed} hovered={hovered} />
      {/* Three-dot menu — hidden until the card is hovered (or the menu itself
          is open). Position absolute so it overlays the card chrome instead
          of taking layout space. */}
      <div
        hidden={accessProvisioning}
        className={cn(
          "absolute top-2 right-2 z-[3] transition-opacity duration-150",
          menuVisible ? "opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
        }}
      >
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="flex h-7 w-7 items-center justify-center rounded-sm text-foreground transition-colors hover:bg-accent/60"
              aria-label="Agent options"
            >
              <EllipsisHorizontalIcon className="h-4 w-4" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[180px] rounded-[10px] p-0">
            <DropdownMenuItem
              asChild
              className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
            >
              <Link to={`/${account}/${name}`} onClick={() => setMenuOpen(false)}>
                <BookOpenIcon className="h-4 w-4" />
                View blueprint
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => {
                setMenuOpen(false);
                setShareOpen(true);
              }}
              className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
            >
              <ShareIcon className="h-4 w-4" />
              Share agent badge
            </DropdownMenuItem>
            {deploymentId && (
              <DropdownMenuItem
                onSelect={copyId}
                className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
              >
                {copied ? <CheckIcon className="h-4 w-4" /> : <DocumentDuplicateIcon className="h-4 w-4" />}
                {copied ? "Copied!" : "Copy deploy ID"}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div className="relative z-[1] h-16 w-16 rounded-md shadow-[0_6px_16px_-4px_rgb(0_0_0_/_0.26),0_2px_4px_-1px_rgb(0_0_0_/_0.12)]">
        <AvatarImage
          src={avatarUrl ?? getAgentAvatarUrl(account, name)}
          alt={name}
          size={64}
          className="block h-16 w-16 rounded-md"
        />
        {/* Border in its own layer so its mix-blend-overlay applies only to the
            border pixels, not the avatar image itself. The semi-transparent
            stroke picks up the gradient + starfield behind it for a "frosted"
            integrated edge. */}
        <div className="pointer-events-none absolute inset-0 rounded-md border-[0.75px] border-foreground/45 mix-blend-overlay" />
      </div>
      <div className="relative z-[1] flex flex-col items-center gap-1">
        <p className="text-heading-2 text-balance text-center text-foreground">{agentTitleDisplay}</p>
        <Link
          to={`/${account}/${name}`}
          className="text-body-sm text-muted-foreground hover:text-foreground hover:underline"
        >
          {account}/{name}
        </Link>
        {accessProvisioning && (
          <div className="mt-1">
            <StatusBadge
              color={accessProvisioningStalled ? "error" : "warning"}
              indicator
              spinning={!accessProvisioningStalled}
            >
              {accessProvisioningStalled
                ? "Access setup needs attention"
                : accessProvisioningDelayed
                  ? "Still setting up access"
                  : "Setting up access"}
            </StatusBadge>
          </div>
        )}
        {!accessProvisioning && (hasError || hasUpdateAvailable) && (
          <div className="mt-1 flex flex-wrap items-center justify-center gap-1.5">
            {hasError && <StatusBadge color="error">Error</StatusBadge>}
            {hasUpdateAvailable &&
              (latestBuildId && deploymentId ? (
                <Link
                  to={`${deploymentConfigurePath(account, deploymentId)}?build=${latestBuildId}`}
                  className="inline-flex rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <StatusBadge color="primary" className="cursor-pointer hover:brightness-110">
                    Update available
                  </StatusBadge>
                </Link>
              ) : (
                <StatusBadge color="primary">Update available</StatusBadge>
              ))}
          </div>
        )}
      </div>
      {/* Middle band: grows to absorb any extra card height, vertically
          centering the sparkline (which keeps its fixed height). When the
          card has no requestSeries this region is empty but still expands,
          so the action row below stays pinned to the bottom. The sparkline
          itself isn't wrapped in a Link — the whole card is the click target
          for the default deployment detail (see `detailPath` / `handleCardClick`). */}
      <div className="relative z-[1] flex w-full flex-1 items-center justify-center">
        {accessProvisioning ? (
          <div className="flex w-full flex-col items-center gap-2 px-2 text-center">
            <p className="text-body-sm text-muted-foreground">
              {accessProvisioningStalled
                ? "We couldn’t finish setting up secure access."
                : accessProvisioningDelayed
                  ? "Secure access is taking longer than usual."
                  : "Securing this deployment before it can be opened."}
            </p>
            {!accessProvisioningStalled && (
              <div className="h-1 w-full max-w-36 overflow-hidden rounded-full bg-muted">
                <div className="h-full w-1/2 rounded-full bg-warning/70 motion-safe:animate-pulse" />
              </div>
            )}
          </div>
        ) : (
          <RequestSparkline
            // When the cache hasn't populated for this deployment yet (or the
            // entry came back without a series), render a flat zero line
            // instead of an empty space so the layout stays consistent across
            // cards in the same row.
            series={requestSeries && requestSeries.length > 1 ? requestSeries : ZERO_SERIES}
            tokenSeries={tokenSeries}
          />
        )}
      </div>
      <div className="relative z-[1] flex w-full items-center gap-2">
        {accessProvisioning ? (
          accessProvisioningStalled ? (
            <Button
              variant="outline"
              className="flex-1 gap-2"
              onClick={onRetryAccess}
              disabled={!onRetryAccess}
            >
              <ArrowPathIcon className="size-4" />
              Retry access setup
            </Button>
          ) : (
            <div
              role="status"
              aria-label="Deployment access is being configured"
              className="flex min-h-10 flex-1 items-center justify-center gap-2 rounded-md border border-border bg-background/35 px-3 text-sm font-medium text-muted-foreground"
            >
              <ArrowPathIcon className="size-4 motion-safe:animate-spin" />
              Updates automatically
            </div>
          )
        ) : canLaunch && deploymentId ? (
          <>
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button asChild variant="outline" size="icon" aria-label="Manage agent" className="size-10">
                    <Link to={deploymentPath(account, deploymentId)}>
                      <Cog6ToothIcon />
                    </Link>
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Manage agent</TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <TooltipProvider delayDuration={0}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="flex-1">
                    <Button
                      asChild={!launchDisabled}
                      disabled={launchDisabled}
                      className="w-full"
                    >
                      {launchDisabled ? (
                        <>
                          Launch
                          <ArrowUpRightIcon strokeWidth={3} className="size-3.5" />
                        </>
                      ) : (
                        <Link to={chatDeploymentPath(deploymentId)}>
                          Launch
                          <ArrowUpRightIcon strokeWidth={3} className="size-3.5" />
                        </Link>
                      )}
                    </Button>
                  </span>
                </TooltipTrigger>
                {launchDisabled && (
                  <TooltipContent className="max-w-[240px] py-1.5" collisionPadding={8}>
                    {getLaunchDisabledMessage(deploymentStatus)}
                  </TooltipContent>
                )}
              </Tooltip>
            </TooltipProvider>
          </>
        ) : (
          <Button asChild variant="outline" className="flex-1">
            {deploymentId ? (
              <Link to={deploymentPath(account, deploymentId)}>
                <Cog6ToothIcon />
                Manage agent
              </Link>
            ) : (
              <button type="button">
                <Cog6ToothIcon />
                Manage agent
              </button>
            )}
          </Button>
        )}
      </div>
      <TradingCardModal
        open={shareOpen}
        onOpenChange={setShareOpen}
        data={cardData}
        avatarColors={avatarColors}
        integrations={integrations}
      />
    </div>
  );
}
