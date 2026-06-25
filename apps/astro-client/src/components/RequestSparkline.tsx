import { useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { motion, useSpring, useTransform } from "motion/react";
import { curveBasis, line as d3Line } from "d3-shape";

const SPARK_W = 240;
const SPARK_H = 48;
const SPARK_PAD = 4;

// Stable empty-state series. Used when the obs cache hasn't populated for a
// deployment yet — renders as a flat midline instead of leaving an empty gap.
export const ZERO_SERIES: number[] = Array.from({ length: 30 }, () => 0);

// Tight spring config: high stiffness for snappy response, enough damping that
// the tooltip doesn't bounce noticeably when the mouse changes direction.
const TOOLTIP_SPRING = { stiffness: 700, damping: 38, mass: 0.4 } as const;
// Pixels above the cursor for the tooltip's bottom edge.
const TOOLTIP_CURSOR_GAP = 12;

const numberFormatter = new Intl.NumberFormat("en-US");

// curveBasis is a cubic B-spline — it doesn't pass through the anchor points
// but follows their shape with very smooth tangents. Ideal for a decorative
// trend line where exact fidelity doesn't matter.
const buildPath = d3Line<{ x: number; y: number }>()
  .x((p) => p.x)
  .y((p) => p.y)
  .curve(curveBasis);

// Each series is independently normalized to fill the same y-range so they
// overlay nicely regardless of magnitude (requests and tokens are usually
// orders of magnitude apart).
function normalizeSeries(series: number[]): { x: number; y: number }[] {
  const max = Math.max(...series, 1);
  const min = Math.min(...series, 0);
  const range = max - min || 1;
  const rawPoints = series.map((v, i) => ({
    x: SPARK_PAD + (i * (SPARK_W - SPARK_PAD * 2)) / (series.length - 1 || 1),
    y: SPARK_H - SPARK_PAD - ((v - min) / range) * (SPARK_H - SPARK_PAD * 2),
  }));
  // Recenter the line so its vertical midline sits at SPARK_H / 2. Without this
  // a constant series anchors to the bottom of the SVG instead of the center.
  const ys = rawPoints.map((p) => p.y);
  const shift = SPARK_H / 2 - (Math.min(...ys) + Math.max(...ys)) / 2;
  return rawPoints.map((p) => ({ x: p.x, y: p.y + shift }));
}

/**
 * Decorative request/token trend line with a hover tooltip. Shared by the
 * deployed-agent cards (org/agents) and the chat inspector so both render the
 * exact same graph.
 */
export function RequestSparkline({
  series,
  tokenSeries,
}: {
  series: number[];
  tokenSeries?: number[];
}) {
  const gradientId = useId();
  const svgRef = useRef<SVGSVGElement>(null);
  const sessionStartedRef = useRef(false);
  const [activeIdx, setActiveIdx] = useState<number | null>(null);
  const cursorX = useSpring(0, TOOLTIP_SPRING);
  const cursorY = useSpring(0, TOOLTIP_SPRING);
  // Translate the y motion value up by the cursor gap; using a transformed
  // motion value keeps the spring driving the underlying position so the
  // tooltip's bottom anchor sits TOOLTIP_CURSOR_GAP px above the cursor.
  const cursorYAdjusted = useTransform(cursorY, (v) => v - TOOLTIP_CURSOR_GAP);
  const requestPoints = normalizeSeries(series);
  const tokenPoints =
    tokenSeries && tokenSeries.length > 1 ? normalizeSeries(tokenSeries) : null;
  const handleMove = (e: React.MouseEvent<SVGSVGElement>) => {
    const svg = svgRef.current;
    if (!svg) return;
    const rect = svg.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    const idx = Math.round(ratio * (series.length - 1));
    if (sessionStartedRef.current) {
      // Smoothly chase the cursor via spring during the hover session.
      cursorX.set(e.clientX);
      cursorY.set(e.clientY);
    } else {
      // First move of a hover session — snap so the tooltip doesn't fly in
      // from its previous position (or from 0,0 on initial mount).
      cursorX.jump(e.clientX);
      cursorY.jump(e.clientY);
      sessionStartedRef.current = true;
    }
    setActiveIdx(idx);
  };
  const handleLeave = () => {
    sessionStartedRef.current = false;
    setActiveIdx(null);
  };
  return (
    <>
      <svg
        ref={svgRef}
        viewBox={`0 0 ${SPARK_W} ${SPARK_H}`}
        preserveAspectRatio="none"
        className="relative z-[1] h-12 w-full"
        onMouseMove={handleMove}
        onMouseLeave={handleLeave}
        aria-hidden="true"
      >
        <defs>
          <linearGradient
            id={gradientId}
            gradientUnits="userSpaceOnUse"
            x1="0"
            y1="0"
            x2={SPARK_W}
            y2="0"
          >
            <stop offset="0%" stopColor="var(--color-indigo-500)" />
            <stop offset="100%" stopColor="var(--color-teal-400)" />
          </linearGradient>
        </defs>
        <path
          d={buildPath(requestPoints) ?? ""}
          fill="none"
          stroke={`url(#${gradientId})`}
          strokeWidth={2.25}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {tokenPoints && (
          <path
            d={buildPath(tokenPoints) ?? ""}
            fill="none"
            stroke={`url(#${gradientId})`}
            strokeWidth={1.5}
            strokeDasharray="1.5 4"
            strokeLinecap="round"
            strokeLinejoin="round"
            opacity={0.75}
          />
        )}
      </svg>
      {activeIdx !== null &&
        typeof document !== "undefined" &&
        createPortal(
          <motion.div
            className="pointer-events-none fixed z-50 rounded-sm border border-border bg-popover px-2 py-1 text-mono-sm whitespace-nowrap shadow-md"
            style={{
              left: cursorX,
              top: cursorYAdjusted,
              translateX: "-50%",
              translateY: "-100%",
            }}
          >
            <div className="text-foreground">
              {numberFormatter.format(series[activeIdx])} requests
            </div>
            {tokenSeries && tokenSeries[activeIdx] !== undefined && (
              <div className="text-faint-foreground">
                {numberFormatter.format(tokenSeries[activeIdx])} tokens
              </div>
            )}
          </motion.div>,
          document.body,
        )}
    </>
  );
}
