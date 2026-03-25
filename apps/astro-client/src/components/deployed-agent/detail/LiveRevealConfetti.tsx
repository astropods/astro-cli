import { useEffect, useRef } from "react";

export function LiveRevealConfetti() {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;

    const colors = ["#15827d", "#57c4c1", "#D48F1E", "#F0816A", "#073d3c", "#c4b89e", "#2d7a4f"];
    const pieces: {
      x: number;
      y: number;
      vx: number;
      vy: number;
      rot: number;
      vr: number;
      w: number;
      h: number;
      color: string;
      shape: "rect" | "circle";
    }[] = [];

    for (let i = 0; i < 120; i += 1) {
      pieces.push({
        x: Math.random() * canvas.width,
        y: -10 - Math.random() * 200,
        vx: (Math.random() - 0.5) * 3.1,
        vy: 2.1 + Math.random() * 3.6,
        rot: Math.random() * Math.PI * 2,
        vr: (Math.random() - 0.5) * 0.14,
        w: 6 + Math.random() * 8,
        h: 4 + Math.random() * 6,
        color: colors[Math.floor(Math.random() * colors.length)],
        shape: Math.random() > 0.5 ? "rect" : "circle",
      });
    }

    let raf = 0;
    const draw = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      let alive = false;

      for (const piece of pieces) {
        piece.x += piece.vx;
        piece.y += piece.vy;
        piece.rot += piece.vr;
        piece.vy += 0.045;
        if (piece.y < canvas.height + 20) alive = true;

        ctx.save();
        ctx.translate(piece.x, piece.y);
        ctx.rotate(piece.rot);
        ctx.fillStyle = piece.color;
        ctx.globalAlpha = Math.max(0, 1 - piece.y / canvas.height);
        if (piece.shape === "circle") {
          ctx.beginPath();
          ctx.arc(0, 0, piece.w / 2, 0, Math.PI * 2);
          ctx.fill();
        } else {
          ctx.fillRect(-piece.w / 2, -piece.h / 2, piece.w, piece.h);
        }
        ctx.restore();
      }

      if (alive) raf = window.requestAnimationFrame(draw);
    };

    const timer = window.setTimeout(() => {
      raf = window.requestAnimationFrame(draw);
    }, 180);

    return () => {
      window.clearTimeout(timer);
      window.cancelAnimationFrame(raf);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      className="pointer-events-none absolute inset-0 z-0"
    />
  );
}
