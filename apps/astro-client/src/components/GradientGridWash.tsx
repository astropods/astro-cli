import { useId } from "react";
import { cn } from "@/lib/utils";
import type { AvatarColors } from "@/lib/api";

interface GradientGridWashProps {
  colors?: AvatarColors;
  /** 0–1 overall opacity multiplier. Defaults to 1. */
  opacity?: number;
  /** Override the container className (position, size, mask). */
  className?: string;
}

/**
 * Decorative gradient wash with grid lines, used as a page background.
 * Renders a radial color gradient masked with an SVG grid pattern.
 * Adapts to light/dark mode independently.
 */
export function GradientGridWash({ colors, opacity = 1, className }: GradientGridWashProps) {
  const id = useId().replace(/:/g, "");
  const glowColor = colors?.glow ?? "var(--color-teal-500)";
  const baseColor = colors?.base ?? "var(--color-teal-500)";
  const gridColor = colors?.vibrant_light ?? "var(--color-teal-700)";
  const lightId = `ggw-light-${id}`;
  const darkId = `ggw-dark-${id}`;

  return (
    <div
      className={cn(
        "pointer-events-none absolute inset-x-0 top-0 z-0 h-[500px] [mask-image:radial-gradient(ellipse_80%_120%_at_25%_0%,black_0%,transparent_70%)]",
        className,
      )}
      style={opacity !== 1 ? { opacity } : undefined}
    >
      {/* Light-mode color wash */}
      <div
        className="absolute inset-0 dark:hidden"
        style={{
          background: `radial-gradient(ellipse 80% 70% at 25% 0%, color-mix(in oklch, ${glowColor} 25%, transparent) 0%, transparent 80%)`,
        }}
      />
      {/* Dark-mode color wash */}
      <div
        className="absolute inset-0 hidden dark:block"
        style={{
          background: `radial-gradient(ellipse 80% 70% at 25% 0%, color-mix(in oklch, ${baseColor} 24%, transparent) 0%, transparent 80%)`,
        }}
      />
      {/* Light-mode grid */}
      <svg className="absolute inset-0 h-full w-full dark:hidden" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id={lightId} width="8" height="8" patternUnits="userSpaceOnUse">
            <path d="M 8 0 L 0 0 0 8" fill="none" stroke={gridColor} strokeWidth="0.75" strokeOpacity="0.35" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill={`url(#${lightId})`} />
      </svg>
      {/* Dark-mode grid */}
      <svg className="absolute inset-0 hidden h-full w-full dark:block" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id={darkId} width="8" height="8" patternUnits="userSpaceOnUse">
            <path d="M 8 0 L 0 0 0 8" fill="none" stroke="white" strokeWidth="0.75" strokeOpacity="0.4" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill={`url(#${darkId})`} />
      </svg>
    </div>
  );
}
