import { useEffect, useMemo, useState } from "react";
import { max } from "d3-array";
import { forceCollide, forceSimulation, forceX, forceY } from "d3-force";
import { scaleSqrt } from "d3-scale";
import { AnimatePresence, motion, useMotionValue, useSpring, type MotionValue } from "motion/react";
import { ArrowsRightLeftIcon, EllipsisHorizontalIcon, GlobeAltIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { getIntegrationIconUrl } from "@/lib/assets";
import { AvatarImage } from "@/components/AvatarImage";
import { formatBytes } from "@/lib/format-utils";
import { formatCompactNumber } from "@/components/agent-detail/charts/chart-utils";
import { useResolvedTheme } from "@/lib/theme";
import { useContainerSize } from "@/hooks/use-container-size";
import {
  destinationLabel,
  groupDestinations,
  groupIconId,
  routeLabel,
  type DestinationGroup,
} from "./destination-groups";
import type { NetworkFlow } from "@/lib/api";

export interface NetworkFlowGraphProps {
  /** Rendered on the left, flowing inward. */
  inbound: NetworkFlow[];
  /** Rendered on the right, flowing outward. */
  outbound: NetworkFlow[];
  agentAvatarUrl?: string;
  height?: number;
  className?: string;
  /** Past this, the tail collapses into one aggregate bubble. Defaults to 20. */
  maxBubblesPerSide?: number;
}

type BubblePayload =
  | { kind: "route"; flow: NetworkFlow }
  | { kind: "destination"; group: DestinationGroup }
  | { kind: "aggregate"; side: "left" | "right"; count: number; avgValue: number };

// Absolute clamps; the usable range is fitted per zone by fitRadiusRange().
const ABS_MIN_R = 3;
const ABS_MAX_R = 64;
const PACK_DENSITY = 0.4;
const MIN_MAX_RATIO = 0.3;
const CENTER_SIZE = 76;
const CENTER_HALF = CENTER_SIZE / 2;
const ICON_RATIO = 0.55;
const CURVE_OFFSET = 14;

// Smaller bubbles still render, they just don't get a connecting line.
const MAX_CABLES_PER_SIDE = 20;
const DEFAULT_MAX_BUBBLES_PER_SIDE = 20;

const MAX_POPOVER_HOSTS = 5;

const MIN_LABEL_FONT = 7;
const MAX_LABEL_FONT = 13;
const MIN_LABEL_CHARS = 3;
const MONO_CHAR_RATIO = 0.6;

const POPOVER_SPRING = { stiffness: 600, damping: 32, mass: 0.4 };
// Kept in step with the card's `max-w-[280px]`, which Tailwind needs literal.
const POPOVER_MAX_WIDTH = 280;

// Below this the zones collide with the centre tile. Matches the sibling grids.
const MIN_GRAPH_WIDTH = 540;

/** formatCompactNumber passes sub-1000 values through, mantissa and all. */
function formatAverage(n: number): string {
  return formatCompactNumber(Math.round(n * 100) / 100);
}



type Bubble = {
  id: string;
  side: "left" | "right";
  r: number;
  iconId: string | null;
  label: string;
  payload: BubblePayload;
  x: number;
  y: number;
};

type SideItem = {
  id: string;
  value: number;
  iconId: string | null;
  label: string;
  payload: BubblePayload;
};

/** Keeps the top (cap-1) by value; the rest become one averaged entry. */
function truncateForDisplay(
  items: SideItem[],
  side: "left" | "right",
  cap: number,
): SideItem[] {
  if (cap <= 0 || items.length <= cap) return items;
  const sorted = [...items].sort((a, b) => b.value - a.value);
  const kept = sorted.slice(0, cap - 1);
  const rest = sorted.slice(cap - 1);
  const avgValue = rest.reduce((sum, x) => sum + x.value, 0) / rest.length;
  kept.push({
    id: `__aggregate_${side}__:${rest.length}`,
    value: avgValue,
    iconId: null,
    label: `+${rest.length} more`,
    payload: { kind: "aggregate", side, count: rest.length, avgValue },
  });
  return kept;
}

function toBubble(
  item: SideItem,
  index: number,
  side: "left" | "right",
  cx: number,
  cy: number,
  r: number,
): Bubble {
  return {
    id: item.id,
    side,
    r,
    iconId: item.iconId,
    label: item.label,
    payload: item.payload,
    x: cx + Math.cos(index) * 8,
    y: cy + Math.sin(index * 2) * 16,
  };
}

export function NetworkFlowGraph({
  inbound,
  outbound,
  agentAvatarUrl,
  height = 360,
  className,
  maxBubblesPerSide = DEFAULT_MAX_BUBBLES_PER_SIDE,
}: NetworkFlowGraphProps) {
  const { ref, width } = useContainerSize<HTMLDivElement>();
  const theme = useResolvedTheme();
  const [bubbles, setBubbles] = useState<Bubble[]>([]);
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const mouseX = useMotionValue(0);
  const mouseY = useMotionValue(0);
  const springX = useSpring(mouseX, POPOVER_SPRING);
  const springY = useSpring(mouseY, POPOVER_SPRING);

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!ref.current) return;
    const rect = ref.current.getBoundingClientRect();
    // An overhang would flicker a scrollbar: overflow-y-auto computes x to auto.
    const half = POPOVER_MAX_WIDTH / 2;
    const x = e.clientX - rect.left;
    mouseX.set(Math.min(Math.max(x, half), Math.max(half, rect.width - half)));
    mouseY.set(e.clientY - rect.top);
  };


  useEffect(() => {
    // Matches the render gate below; don't simulate what won't be painted.
    if (!height || width < MIN_GRAPH_WIDTH) return;

    const leftCx = width * 0.18;
    const rightCx = width * 0.82;
    const cy = height / 2;

    // Route paths never resolve to a brand icon, so only outbound looks up.
    const leftItems = truncateForDisplay(
      inbound.map<SideItem>((f) => ({
        id: `in-${f.peer}`,
        value: f.request_count,
        iconId: null,
        label: f.peer,
        payload: { kind: "route", flow: f },
      })),
      "left",
      maxBubblesPerSide,
    );
    const rightItems = truncateForDisplay(
      groupDestinations(outbound).map<SideItem>((group) => ({
        id: `out-${group.domain}`,
        value: group.requestCount,
        iconId: groupIconId(group),
        label: group.domain,
        payload: { kind: "destination", group },
      })),
      "right",
      maxBubblesPerSide,
    );

    const zoneArea = (width / 3) * height;
    const leftRange = fitRadiusRange(leftItems.length, zoneArea);
    const rightRange = fitRadiusRange(rightItems.length, zoneArea);

    const radiusLeft = scaleSqrt()
      .domain([0, max(leftItems, (s) => s.value) ?? 1])
      .range(leftRange);

    const radiusRight = scaleSqrt()
      .domain([0, max(rightItems, (s) => s.value) ?? 1])
      .range(rightRange);

    const nodes: Bubble[] = [
      ...leftItems.map((s, i) => toBubble(s, i, "left", leftCx, cy, radiusLeft(s.value))),
      ...rightItems.map((s, i) => toBubble(s, i, "right", rightCx, cy, radiusRight(s.value))),
    ];

    const sim = forceSimulation<Bubble>(nodes)
      .force("x", forceX<Bubble>((d) => (d.side === "left" ? leftCx : rightCx)).strength(0.25))
      .force("y", forceY<Bubble>(cy).strength(0.12))
      .force("collide", forceCollide<Bubble>((d) => d.r + Math.max(1.5, d.r * 0.08)).strength(0.9))
      .stop();

    for (let i = 0; i < 300; i++) sim.tick();
    setBubbles(nodes.slice());
  }, [inbound, outbound, width, height, maxBubblesPerSide]);

  const centerX = width / 2;
  const centerY = height / 2;

  const geometries = useMemo(
    () => bubbles.map((b) => ({ bubble: b, geom: flowGeometry(centerX, centerY, b) })),
    [bubbles, centerX, centerY],
  );

  const cabledIds = useMemo(() => {
    const pickTop = (side: "left" | "right") =>
      bubbles
        .filter((b) => b.side === side)
        .sort((a, b) => b.r - a.r)
        .slice(0, MAX_CABLES_PER_SIDE)
        .map((b) => b.id);
    return new Set([...pickTop("left"), ...pickTop("right")]);
  }, [bubbles]);

  const cables = useMemo(
    () => geometries.filter((g) => cabledIds.has(g.bubble.id)),
    [geometries, cabledIds],
  );

  const tooNarrow = width > 0 && width < MIN_GRAPH_WIDTH;

  return (
    <div
      ref={ref}
      // Zero height, not display:none, or the observer stops reporting a width.
      className={cn("relative w-full", tooNarrow ? "overflow-hidden" : className)}
      style={{ height: tooNarrow ? 0 : height }}
      onMouseMove={handleMouseMove}
    >
      {width >= MIN_GRAPH_WIDTH && (
        <svg
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          className="overflow-visible"
          // Decorative: the flows table below carries every peer accessibly.
          aria-hidden="true"
        >
          <g className="text-border" stroke="currentColor" fill="none" strokeWidth={1.25}>
            {cables.map(({ bubble, geom }) => (
              <path
                key={`flow-${bubble.id}`}
                d={pathFromGeometry(geom)}
                opacity={0.7}
              />
            ))}
          </g>

          {bubbles.map((b) => (
            <BubbleNode
              key={b.id}
              bubble={b}
              theme={theme}
              onHoverChange={setHoveredId}
            />
          ))}

          <g transform={`translate(${centerX}, ${centerY})`}>
            <foreignObject
              x={-CENTER_HALF}
              y={-CENTER_HALF}
              width={CENTER_SIZE}
              height={CENTER_SIZE}
              className="pointer-events-none"
            >
              {agentAvatarUrl ? (
                <AvatarImage
                  src={agentAvatarUrl}
                  alt=""
                  size={CENTER_SIZE}
                  className="h-full w-full rounded-lg"
                />
              ) : (
                <div className="h-full w-full rounded-lg bg-primary/15" aria-hidden />
              )}
            </foreignObject>
          </g>
        </svg>
      )}
      <BubblePopover
        bubble={bubbles.find((b) => b.id === hoveredId) ?? null}
        springX={springX}
        springY={springY}
      />
    </div>
  );
}

function BubblePopover({
  bubble,
  springX,
  springY,
}: {
  bubble: Bubble | null;
  springX: MotionValue<number>;
  springY: MotionValue<number>;
}) {
  return (
    <AnimatePresence>
      {bubble && (
        <motion.div
          className="pointer-events-none absolute left-0 top-0 z-20"
          style={{ x: springX, y: springY }}
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ opacity: { duration: 0.12 } }}
        >
          <div className="-mt-3 w-max max-w-[280px] -translate-x-1/2 -translate-y-full">
            <div className="rounded-md border border-border bg-popover px-3 py-2 shadow-md">
              <BubblePopoverContent bubble={bubble} />
            </div>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function BubblePopoverContent({ bubble }: { bubble: Bubble }) {
  const theme = useResolvedTheme();
  const { payload } = bubble;

  let title: string;
  let detail: string;
  if (payload.kind === "route") {
    const { flow } = payload;
    title = flow.peer;
    detail = `${formatCompactNumber(flow.request_count)} requests · ${formatBytes(flow.bytes_total)}`;
  } else if (payload.kind === "destination") {
    const { group } = payload;
    // A lone host names itself; the merged domain would hide detail.
    title = group.hosts.length === 1 ? group.hosts[0].peer : group.domain;
    detail = `${formatCompactNumber(group.requestCount)} requests · ${formatBytes(group.bytesTotal)}`;
  } else {
    title = `+${payload.count} more ${payload.side === "left" ? "routes" : "destinations"}`;
    detail = `Avg ${formatAverage(payload.avgValue)} requests`;
  }

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-[5px] text-body-sm font-medium text-foreground">
        <PopoverIcon bubble={bubble} theme={theme} />
        <span className="min-w-0 truncate">{title}</span>
      </div>
      <div className="text-body-sm text-muted-foreground">{detail}</div>
      {payload.kind === "destination" && payload.group.hosts.length > 1 && (
        <GroupedHostList group={payload.group} />
      )}
    </div>
  );
}

function GroupedHostList({ group }: { group: DestinationGroup }) {
  const shown = group.hosts.slice(0, MAX_POPOVER_HOSTS);
  const hiddenCount = group.hosts.length - shown.length;

  return (
    <div className="mt-1.5 flex flex-col gap-0.5 border-t border-border pt-1.5">
      {shown.map((host) => (
        <div key={host.peer} className="flex items-baseline gap-3 text-body-sm">
          <span className="min-w-0 grow truncate text-muted-foreground">{host.peer}</span>
          <span className="shrink-0 tabular-nums text-foreground">
            {formatCompactNumber(host.request_count)}
          </span>
        </div>
      ))}
      {hiddenCount > 0 && (
        <div className="text-body-sm text-faint-foreground">
          +{hiddenCount} more {hiddenCount === 1 ? "host" : "hosts"}
        </div>
      )}
    </div>
  );
}

function PopoverIcon({ bubble, theme }: { bubble: Bubble; theme: "light" | "dark" }) {
  if (bubble.payload.kind === "aggregate") return null;
  return (
    <div className="grid h-3 w-3 shrink-0 place-items-center">
      {bubble.iconId ? (
        <img
          src={getIntegrationIconUrl(bubble.iconId, theme)}
          alt=""
          className="block h-full w-full object-contain dark:brightness-150"
        />
      ) : bubble.side === "left" ? (
        <ArrowsRightLeftIcon className="h-full w-full text-muted-foreground" />
      ) : (
        <GlobeAltIcon className="h-full w-full text-muted-foreground" />
      )}
    </div>
  );
}

type BubbleIconKind = "aggregate" | "integration" | "route" | "destination";

function bubbleIconKind(bubble: Bubble): BubbleIconKind {
  if (bubble.payload.kind === "aggregate") return "aggregate";
  if (bubble.iconId) return "integration";
  return bubble.side === "left" ? "route" : "destination";
}

function BubbleNode({
  bubble,
  theme,
  onHoverChange,
}: {
  bubble: Bubble;
  theme: "light" | "dark";
  onHoverChange: (id: string | null) => void;
}) {
  const iconSize = bubble.r * 2 * ICON_RATIO;
  const showIcon = bubble.r >= 10;
  const isAggregate = bubble.payload.kind === "aggregate";
  const iconStyle = { width: iconSize, height: iconSize };
  const kind = bubbleIconKind(bubble);

  // No icon tells these apart, so fit text to the circle's widest chord.
  const fontSize = Math.min(MAX_LABEL_FONT, Math.max(MIN_LABEL_FONT, bubble.r * 0.42));
  const labelChars = Math.floor((bubble.r * 1.7) / (fontSize * MONO_CHAR_RATIO));
  const isTextual = kind === "route" || kind === "destination";
  const showLabel = isTextual && bubble.r >= 10 && labelChars >= MIN_LABEL_CHARS;
  const label =
    kind === "route"
      ? routeLabel(bubble.label, labelChars)
      : destinationLabel(bubble.label, labelChars);

  return (
    <g
      transform={`translate(${bubble.x}, ${bubble.y})`}
      onMouseEnter={() => onHoverChange(bubble.id)}
      onMouseLeave={() => onHoverChange(null)}
      className="cursor-default"
    >
      <circle r={Math.max(bubble.r + 4, 10)} fill="transparent" />
      <circle
        r={bubble.r}
        className={cn("stroke-border", isAggregate ? "fill-muted" : "fill-card")}
        strokeWidth={bubble.r >= 8 ? 1.5 : 1}
        strokeDasharray={isAggregate ? "3 3" : undefined}
      />
      {showLabel && (
        <text
          textAnchor="middle"
          dominantBaseline="central"
          fontSize={fontSize}
          className="pointer-events-none select-none fill-muted-foreground font-mono"
        >
          {label}
        </text>
      )}
      {showIcon && !isTextual && (
        <foreignObject
          x={-bubble.r}
          y={-bubble.r}
          width={bubble.r * 2}
          height={bubble.r * 2}
          className="pointer-events-none"
        >
          <div className="flex h-full w-full items-center justify-center">
            {kind === "aggregate" && (
              <EllipsisHorizontalIcon style={iconStyle} className="text-muted-foreground" />
            )}
            {kind === "integration" && bubble.iconId && (
              <img
                src={getIntegrationIconUrl(bubble.iconId, theme)}
                alt=""
                style={iconStyle}
                className="object-contain dark:brightness-150"
              />
            )}
          </div>
        </foreignObject>
      )}
    </g>
  );
}

/** Fits the radius range so bubbles occupy ~PACK_DENSITY of the zone area. */
function fitRadiusRange(count: number, zoneArea: number): [number, number] {
  if (count <= 0) return [ABS_MIN_R, ABS_MAX_R];
  const targetArea = zoneArea * PACK_DENSITY;
  const avgR = Math.sqrt(targetArea / (Math.PI * count));
  const maxR = Math.min(ABS_MAX_R, avgR * 1.8);
  const minR = Math.max(ABS_MIN_R, maxR * MIN_MAX_RATIO);
  return [minR, maxR];
}

type FlowGeometry = {
  p0: [number, number];
  p1: [number, number]; // quadratic control point
  p2: [number, number];
};

function flowGeometry(centerX: number, centerY: number, b: Bubble): FlowGeometry {
  const dx = b.x - centerX;
  const dy = b.y - centerY;
  const len = Math.hypot(dx, dy) || 1;
  const ux = dx / len;
  const uy = dy / len;

  const startX = centerX + ux * CENTER_HALF;
  const startY = centerY + uy * CENTER_HALF;
  const endX = b.x - ux * b.r;
  const endY = b.y - uy * b.r;

  const nx = -uy;
  const ny = ux;
  const cx = (startX + endX) / 2 + nx * CURVE_OFFSET;
  const cy = (startY + endY) / 2 + ny * CURVE_OFFSET;

  return { p0: [startX, startY], p1: [cx, cy], p2: [endX, endY] };
}

function pathFromGeometry({ p0, p1, p2 }: FlowGeometry): string {
  return `M ${p0[0]} ${p0[1]} Q ${p1[0]} ${p1[1]} ${p2[0]} ${p2[1]}`;
}
