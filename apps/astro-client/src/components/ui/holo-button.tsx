import { useCallback, useMemo, useRef, cloneElement, isValidElement, type ReactNode, type ReactElement, type PointerEvent as ReactPointerEvent, type ComponentProps } from "react";
import { cn } from "@/lib/utils";
import { deriveAccentColors } from "@/lib/color-utils";
import { Button } from "@/components/ui/button";

const borderGlowStyle = {
  border: "1px solid transparent",
  background: "radial-gradient(circle 80px at var(--px, 50%) var(--py, 50%), rgba(255,255,255,0.7) 0%, rgba(255,255,255,0.1) 40%, transparent 70%) border-box",
  WebkitMask: "linear-gradient(#fff 0 0) padding-box, linear-gradient(#fff 0 0)",
  WebkitMaskComposite: "xor",
  maskComposite: "exclude",
} as const;

const noiseStyle = {
  backgroundImage: `url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='1.5' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E")`,
  backgroundRepeat: "repeat",
  backgroundSize: "200px 200px",
} as const;

const shineStyle = {
  mixBlendMode: "color-dodge" as const,
  backgroundImage: [
    "radial-gradient(circle at var(--px,30%) var(--py,40%), #fff 5%, #000 50%, #fff 80%)",
    "linear-gradient(-45deg, #000 15%, #fff, #000 85%)",
    "repeating-linear-gradient(135deg, hsl(280,80%,50%) 0%, hsl(200,80%,50%) 10%, hsl(140,80%,50%) 20%, hsl(60,80%,50%) 30%, hsl(330,80%,50%) 40%, hsl(280,80%,50%) 50%)",
  ].join(","),
  backgroundBlendMode: "soft-light, difference",
  backgroundSize: "120% 120%, 200% 200%, 150% 150%",
  backgroundPosition: "center center, calc(100% * var(--fl, 0.3)) 50%, center center",
  filter: "brightness(0.5) contrast(1.5) saturate(0.8)",
};

const glareStyle = {
  backgroundImage: "radial-gradient(farthest-corner circle at var(--px,30%) var(--py,40%), hsla(0,0%,100%,0.8) 10%, hsla(0,0%,100%,0.5) 20%, hsla(0,0%,0%,0.75) 90%)",
  filter: "brightness(0.7) contrast(1.5)",
};

function deriveCtaColors(hex: string) {
  const { light, dark } = deriveAccentColors(hex);
  return { base: light, darkBase: dark };
}

const holoOverlays = (
  <>
    <span
      className="pointer-events-none absolute inset-[1px] rounded-[inherit] transition-opacity duration-300"
      style={{ ...borderGlowStyle, opacity: "var(--o, 0.25)" as unknown as number }}
    />
    <span className="pointer-events-none absolute inset-0 opacity-30 mix-blend-overlay" style={noiseStyle} />
    <span
      className="pointer-events-none absolute inset-0 transition-opacity duration-300"
      style={{ ...shineStyle, opacity: "var(--ho, 0)" as unknown as number }}
    />
    <span
      className="pointer-events-none absolute inset-0 mix-blend-overlay transition-opacity duration-300"
      style={{ ...glareStyle, opacity: "var(--ho, 0)" as unknown as number }}
    />
  </>
);

export interface HoloButtonProps extends Omit<ComponentProps<typeof Button>, "style" | "ref"> {
  /** Hex color to derive the button palette from (e.g. avatar accent color). */
  accentHex?: string;
  /** Extra padding around the button that activates the proximity border glow. */
  proximityPadding?: number;
  children: ReactNode;
}

export function HoloButton({ accentHex, proximityPadding = 16, className, children, asChild, ...buttonProps }: HoloButtonProps) {
  const lightRef = useRef<HTMLElement | null>(null);
  const darkRef = useRef<HTMLElement | null>(null);

  const colors = useMemo(() => accentHex ? deriveCtaColors(accentHex) : null, [accentHex]);

  const handleProximityMove = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    for (const el of [lightRef.current, darkRef.current]) {
      if (!el) continue;
      const rect = el.getBoundingClientRect();
      const px = ((e.clientX - rect.left) / rect.width) * 100;
      const py = ((e.clientY - rect.top) / rect.height) * 100;
      el.style.setProperty("--px", `${px}%`);
      el.style.setProperty("--py", `${py}%`);
      el.style.setProperty("--fl", String(px / 100));
      el.style.setProperty("--o", "0.6");
    }
  }, []);

  const handleProximityLeave = useCallback(() => {
    lightRef.current?.style.setProperty("--o", "0.25");
    darkRef.current?.style.setProperty("--o", "0.25");
  }, []);

  const handleBtnEnter = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    e.currentTarget.style.setProperty("--ho", "0.6");
  }, []);

  const handleBtnLeave = useCallback((e: ReactPointerEvent<HTMLElement>) => {
    e.currentTarget.style.setProperty("--ho", "0");
  }, []);

  if (!colors) {
    return <Button className={className} asChild={asChild} {...buttonProps}>{children}</Button>;
  }

  // When asChild, inject overlays into the child element's children
  const renderContent = (ref: React.RefObject<HTMLElement | null>, bgColor: string, hideClass: string) => {
    const btnClassName = cn("relative overflow-hidden text-white", hideClass, className);

    if (asChild && isValidElement(children)) {
      const child = children as ReactElement<{ className?: string; children?: ReactNode }>;
      return (
        <Button
          asChild
          className={btnClassName}
          style={{ backgroundColor: bgColor }}
          {...buttonProps}
        >
          {cloneElement(child, {
            ref,
            onPointerEnter: handleBtnEnter,
            onPointerLeave: handleBtnLeave,
          } as Record<string, unknown>, holoOverlays, child.props.children)}
        </Button>
      );
    }

    return (
      <Button
        ref={ref as React.RefObject<HTMLButtonElement>}
        className={btnClassName}
        style={{ backgroundColor: bgColor }}
        onPointerEnter={handleBtnEnter}
        onPointerLeave={handleBtnLeave}
        {...buttonProps}
      >
        {holoOverlays}
        {children}
      </Button>
    );
  };

  const pad = proximityPadding;

  return (
    <div
      className="relative w-full"
      onPointerMove={handleProximityMove}
      onPointerLeave={handleProximityLeave}
    >
      {/* Invisible expanded hit area for proximity detection */}
      <div
        className="pointer-events-auto absolute"
        style={{ inset: `-${pad}px` }}
      />
      {renderContent(lightRef, colors.base, "dark:hidden")}
      {renderContent(darkRef, colors.darkBase, "hidden dark:flex")}
    </div>
  );
}
