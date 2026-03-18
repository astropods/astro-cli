import { useCallback, useRef, type ReactNode } from "react";
import { computeHoloVars, HOLO_RESET_VARS } from "astro-trading-card/browser";
import "astro-trading-card/holo.css";

interface HoloCardProps {
  children: ReactNode;
}

export function HoloCard({ children }: HoloCardProps) {
  const cardRef = useRef<HTMLDivElement>(null);

  const onPointerMove = useCallback((e: React.PointerEvent) => {
    const el = cardRef.current;
    if (!el) return;
    const vars = computeHoloVars(el.getBoundingClientRect(), e.clientX, e.clientY);
    for (const [k, v] of Object.entries(vars)) {
      el.style.setProperty(k, v);
    }
  }, []);

  const onPointerLeave = useCallback(() => {
    const el = cardRef.current;
    if (!el) return;
    for (const [k, v] of Object.entries(HOLO_RESET_VARS)) {
      el.style.setProperty(k, v);
    }
  }, []);

  return (
    <div style={{ perspective: 600 }}>
      <div
        ref={cardRef}
        className="holo-card"
        onPointerMove={onPointerMove}
        onPointerLeave={onPointerLeave}
      >
        {children}
        <div className="holo-card__shine" />
        <div className="holo-card__glare" />
      </div>
    </div>
  );
}
