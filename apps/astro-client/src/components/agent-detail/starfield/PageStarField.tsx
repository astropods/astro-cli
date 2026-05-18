import type { CSSProperties } from "react";
import { StarField } from "./StarField";
import { useResolvedTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const BG = {
  dark: "linear-gradient(to top, color-mix(in srgb, var(--color-indigo-900) 18%, var(--color-background)), var(--color-background))",
  light: "linear-gradient(in oklch to bottom, color-mix(in srgb, var(--color-blue-500) 60%, var(--color-surface)) 0%, color-mix(in srgb, var(--color-blue-500) 40%, var(--color-surface)) 70%, color-mix(in srgb, var(--color-blue-500) 25%, var(--color-surface)) 80%, color-mix(in srgb, var(--color-pink-500) 12%, var(--color-surface)) 93%, color-mix(in srgb, var(--color-amber-500) 20%, var(--color-surface)) 100%)",
};
const STARS = { dark: "oklch(87.08% 0.0571 272.201)", light: "#ffffff" };
const CLOUDS = { dark: "oklch(58.40% 0.2055 274.722)", light: "#ffffff" };

interface PageStarFieldProps {
  className?: string;
}

export function PageStarField({ className }: PageStarFieldProps) {
  const isDark = useResolvedTheme() === "dark";
  return (
    <div
      className={cn("[--sf-bg:var(--sf-bg-light)] dark:[--sf-bg:var(--sf-bg-dark)]", className)}
      style={{ "--sf-bg-light": BG.light, "--sf-bg-dark": BG.dark } as CSSProperties}
    >
      <StarField
        backgroundColor="var(--sf-bg)"
        starColor={isDark ? STARS.dark : STARS.light}
        starOpacity={isDark ? 1 : 10}
        starDensity={isDark ? 1 : 2}
        cloudColor={isDark ? CLOUDS.dark : CLOUDS.light}
        cloudOpacity={isDark ? 1 : 6}
        direction="right"
        speed={0.5}
        // Fixed seed so star positions stay stable across renders and route changes;
        // any constant integer works — this one just happens to look good.
        seed={95522}
      />
    </div>
  );
}
