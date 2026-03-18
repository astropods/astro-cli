import { useCallback, useRef, type ReactNode } from "react";

const clamp = (v: number, min = 0, max = 100) =>
  Math.min(max, Math.max(min, v));

interface HoloCardProps {
  children: ReactNode;
}

export function HoloCard({ children }: HoloCardProps) {
  const cardRef = useRef<HTMLDivElement>(null);

  const onPointerMove = useCallback((e: React.PointerEvent) => {
    const el = cardRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const px = clamp(((e.clientX - rect.left) / rect.width) * 100);
    const py = clamp(((e.clientY - rect.top) / rect.height) * 100);
    const cx = px - 50;
    const cy = py - 50;
    const dist = Math.sqrt(cx * cx + cy * cy) / 50;

    const s = el.style;
    s.setProperty("--px", `${px}%`);
    s.setProperty("--py", `${py}%`);
    s.setProperty("--fl", String(px / 100));
    s.setProperty("--ft", String(py / 100));
    s.setProperty("--fc", String(clamp(dist, 0, 1)));
    s.setProperty("--o", "1");
    s.setProperty("--rx", `${-(cx / 4)}deg`);
    s.setProperty("--ry", `${cy / 4}deg`);
  }, []);

  const onPointerLeave = useCallback(() => {
    const el = cardRef.current;
    if (!el) return;
    el.style.setProperty("--o", "0");
    el.style.setProperty("--rx", "0deg");
    el.style.setProperty("--ry", "0deg");
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

        <style>{`
          .holo-card {
            position: relative;
            display: inline-block;
            border-radius: 16px;
            transform-style: preserve-3d;
            transform: rotateY(var(--rx, 0deg)) rotateX(var(--ry, 0deg));
            transition: transform 0.15s ease-out, box-shadow 0.15s ease-out;
            will-change: transform;
            user-select: none;
            -webkit-user-select: none;
            box-shadow: 0 8px 32px rgba(0,0,0,0.5), 0 2px 8px rgba(0,0,0,0.3);
          }
          .holo-card:hover {
            box-shadow: 0 12px 48px rgba(0,0,0,0.6), 0 4px 12px rgba(0,0,0,0.4);
          }
          .holo-card svg { display: block; border-radius: 16px; }
          .holo-card__shine {
            position: absolute; inset: 0; border-radius: 16px; z-index: 2;
            pointer-events: none; mix-blend-mode: color-dodge;
            transition: opacity 0.15s ease-out;
            opacity: 0.4;
            background-image:
              radial-gradient(circle at var(--px,50%) var(--py,50%), #fff 5%, #000 50%, #fff 80%),
              linear-gradient(-45deg, #000 15%, #fff, #000 85%),
              repeating-linear-gradient(135deg,
                hsl(280,80%,50%) 0%, hsl(200,80%,50%) 10%, hsl(140,80%,50%) 20%,
                hsl(60,80%,50%) 30%, hsl(330,80%,50%) 40%, hsl(280,80%,50%) 50%);
            background-blend-mode: soft-light, difference;
            background-size: 120% 120%, 200% 200%, 150% 150%;
            background-position:
              center center,
              calc(100% * var(--fl,0.5)) calc(100% * var(--ft,0.5)),
              center center;
            filter: brightness(0.55) contrast(1.5) saturate(1);
          }
          .holo-card:hover .holo-card__shine {
            opacity: calc(1.5 * var(--o, 0.4) - var(--fc, 0));
          }
          .holo-card__glare {
            position: absolute; inset: 0; border-radius: 16px; z-index: 3;
            pointer-events: none; mix-blend-mode: overlay;
            transition: opacity 0.15s ease-out;
            opacity: 0.3;
            background-image:
              radial-gradient(farthest-corner circle at var(--px,50%) var(--py,50%),
                hsla(0,0%,100%,0.8) 10%, hsla(0,0%,100%,0.5) 20%, hsla(0,0%,0%,0.75) 90%);
            filter: brightness(0.7) contrast(1.5);
          }
          .holo-card:hover .holo-card__glare {
            opacity: var(--o, 0.3);
          }
        `}</style>
      </div>
    </div>
  );
}
