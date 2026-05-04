import { useState, useMemo, useCallback, type ReactNode } from "react";
import { getSuperEllipsePathAsDataUri } from "superellipsejs";
import { cn } from "@/lib/utils";
import { useContainerSize } from "@/hooks/use-container-size";

interface SquircleProps {
  children: ReactNode;
  className?: string;
  /** r1/r2 control the superellipse curve (0–0.5 range). Higher = rounder. */
  r1?: number;
  r2?: number;
  onClick?: () => void;
  selected?: boolean;
  /** Visually dim the tile (e.g. when a sibling is selected). */
  dimmed?: boolean;
}

export function Squircle({
  children,
  className,
  r1 = 0.08,
  r2 = 0.32,
  onClick,
  selected,
  dimmed,
}: SquircleProps) {
  const { ref, width: sizeW, height: sizeH } = useContainerSize();
  const [hovered, setHovered] = useState(false);

  const handleEnter = useCallback(() => setHovered(true), []);
  const handleLeave = useCallback(() => setHovered(false), []);

  const { contentMask, highlightMask } = useMemo(() => {
    if (sizeW === 0 || sizeH === 0) return { contentMask: undefined, borderMask: undefined };
    const minDim = Math.min(sizeW, sizeH);
    const { dataUri } = getSuperEllipsePathAsDataUri(sizeW, sizeH, r1 * minDim, r2 * minDim);
    const mask = {
      maskImage: `url("${dataUri}")`,
      maskSize: "100% 100%",
      WebkitMaskImage: `url("${dataUri}")`,
      WebkitMaskSize: "100% 100%",
    } as React.CSSProperties;

    // Ring mask: outer squircle minus a 2px-smaller inner squircle = 1px border
    const iw = sizeW - 2;
    const ih = sizeH - 2;
    const iMinDim = Math.min(iw, ih);
    const { dataUri: innerUri } = getSuperEllipsePathAsDataUri(iw, ih, r1 * iMinDim, r2 * iMinDim);
    const highlightMask = {
      maskImage: `url("${dataUri}"), url("${innerUri}")`,
      maskSize: "100% 100%, calc(100% - 2px) calc(100% - 2px)",
      maskPosition: "center, center",
      maskRepeat: "no-repeat, no-repeat",
      maskComposite: "subtract",
      WebkitMaskImage: `url("${dataUri}"), url("${innerUri}")`,
      WebkitMaskSize: "100% 100%, calc(100% - 2px) calc(100% - 2px)",
      WebkitMaskPosition: "center, center",
      WebkitMaskRepeat: "no-repeat, no-repeat",
      WebkitMaskComposite: "xor",
    } as React.CSSProperties;

    return { contentMask: mask, highlightMask };
  }, [sizeW, sizeH, r1, r2]);

  return (
    <div
      ref={ref}
      onClick={onClick}
      onMouseEnter={onClick ? handleEnter : undefined}
      onMouseLeave={onClick ? handleLeave : undefined}
      className={cn(
        "drop-shadow-[0_2px_8px_rgba(0,0,0,0.12)] transition-[filter] duration-200 dark:drop-shadow-[0_2px_10px_rgba(0,0,0,0.4)]",
        onClick && "cursor-pointer",
        dimmed && "dark:brightness-[0.7]",
        className,
      )}
    >
      {/* Content fill */}
      <div
        className="relative bg-card transition-[background-color] duration-200"
        style={contentMask}
      >
        {/* Hover tint — darken in light, lighten in dark */}
        <div
          className={cn(
            "pointer-events-none absolute inset-0 transition-opacity duration-200",
            "bg-primary/[0.04] dark:bg-primary/[0.08]",
            hovered || selected ? "opacity-100" : "opacity-0",
          )}
        />
        {/* Inset ring — bottom shadow in light, top highlight in dark */}
        {highlightMask && (
          <div
            className={cn(
              "pointer-events-none absolute inset-0 bg-gradient-to-t to-transparent to-50% transition-colors duration-200",
              "from-border dark:bg-gradient-to-b",
              selected
                ? "dark:from-primary/60"
                : hovered
                  ? "dark:from-primary/50"
                  : "dark:from-border",
            )}
            style={highlightMask}
          />
        )}
        <div className="relative">{children}</div>
      </div>
    </div>
  );
}
