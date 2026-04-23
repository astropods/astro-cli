import type { AvatarColors } from "@/lib/api";

interface GradientGridWashProps {
  colors?: AvatarColors;
}

/**
 * Decorative gradient wash with grid lines, used as a page background.
 * Renders a radial color gradient masked with an SVG grid pattern.
 * Adapts to light/dark mode independently.
 */
export function GradientGridWash({ colors }: GradientGridWashProps) {
  const glowColor = colors?.glow ?? "var(--color-teal-500)";
  const baseColor = colors?.base ?? "var(--color-teal-500)";
  const gridColor = colors?.vibrant_light ?? "var(--color-teal-700)";

  return (
    <div
      className="pointer-events-none absolute inset-x-0 top-0 z-0 h-[500px] [mask-image:radial-gradient(ellipse_80%_120%_at_25%_0%,black_0%,transparent_70%)]"
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
          <pattern id="blueprint-detail-grid-light" width="8" height="8" patternUnits="userSpaceOnUse">
            <path d="M 8 0 L 0 0 0 8" fill="none" stroke={gridColor} strokeWidth="0.75" strokeOpacity="0.35" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#blueprint-detail-grid-light)" />
      </svg>
      {/* Dark-mode grid */}
      <svg className="absolute inset-0 hidden h-full w-full dark:block" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="blueprint-detail-grid-dark" width="8" height="8" patternUnits="userSpaceOnUse">
            <path d="M 8 0 L 0 0 0 8" fill="none" stroke="white" strokeWidth="0.5" strokeOpacity="0.12" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#blueprint-detail-grid-dark)" />
      </svg>
    </div>
  );
}
