import type { DatasetJudgmentVerdict } from "@/lib/api";

type EvalFlightCue = DatasetJudgmentVerdict | "undo";

const EVAL_FLIGHT_META: Record<
  EvalFlightCue,
  { color: string; iconPath: string; rotation: number }
> = {
  good: {
    color: "var(--success)",
    iconPath: "M20 6 9 17l-5-5",
    rotation: 20,
  },
  bad: {
    color: "var(--destructive)",
    iconPath: "M18 6 6 18M6 6l12 12",
    rotation: -20,
  },
  unknown: {
    color: "var(--muted-foreground)",
    iconPath: "M5 12h14",
    rotation: 0,
  },
  undo: {
    color: "var(--primary)",
    iconPath: "M3 7v6h6M21 17a9 9 0 0 0-15-6.7L3 13",
    rotation: -24,
  },
};

function createFlightIcon(pathData: string) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("width", "15");
  svg.setAttribute("height", "15");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "#fff");
  svg.setAttribute("stroke-width", "2.6");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");

  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", pathData);
  svg.appendChild(path);
  return svg;
}

function pulseTarget(
  target: HTMLElement,
  color: string,
  reducedMotion = false,
) {
  if (typeof target.animate !== "function") return;

  const frames: Keyframe[] = reducedMotion
    ? [
        {
          boxShadow: `0 0 0 0 color-mix(in oklch, ${color} 0%, transparent)`,
        },
        {
          boxShadow: `0 0 0 5px color-mix(in oklch, ${color} 24%, transparent)`,
        },
        {
          boxShadow: `0 0 0 0 color-mix(in oklch, ${color} 0%, transparent)`,
        },
      ]
    : [
        {
          transform: "scale(1)",
          boxShadow: `0 0 0 0 color-mix(in oklch, ${color} 0%, transparent)`,
        },
        { transform: "scale(0.86)", offset: 0.18 },
        {
          transform: "scale(1.28)",
          boxShadow: `0 0 0 6px color-mix(in oklch, ${color} 20%, transparent)`,
          offset: 0.52,
        },
        {
          transform: "scale(1)",
          boxShadow: `0 0 0 0 color-mix(in oklch, ${color} 0%, transparent)`,
        },
      ];

  target.animate(frames, {
    duration: reducedMotion ? 260 : 440,
    easing: "cubic-bezier(.16,1,.3,1)",
  });
}

function flyEvalCueToTarget(
  originRect: DOMRect | null,
  target: HTMLElement | null | undefined,
  cue: EvalFlightCue,
) {
  if (
    typeof document === "undefined" ||
    typeof window === "undefined" ||
    !originRect ||
    !target
  ) {
    return;
  }

  const meta = EVAL_FLIGHT_META[cue];
  const reducedMotion =
    window.matchMedia?.("(prefers-reduced-motion: reduce)")?.matches ?? false;

  if (reducedMotion) {
    pulseTarget(target, meta.color, true);
    return;
  }

  const size = 28;
  const targetRect = target.getBoundingClientRect();
  const startX = originRect.left + originRect.width / 2 - size / 2;
  const startY = originRect.top + originRect.height / 2 - size / 2;
  const endX = targetRect.left + targetRect.width / 2 - size / 2;
  const endY = targetRect.top + targetRect.height / 2 - size / 2;
  const dx = endX - startX;
  const dy = endY - startY;
  const distance = Math.hypot(dx, dy);

  const fly = document.createElement("div");
  fly.setAttribute("aria-hidden", "true");
  fly.style.cssText = [
    "position:fixed",
    "left:0",
    "top:0",
    `width:${size}px`,
    `height:${size}px`,
    "z-index:9999",
    "pointer-events:none",
    "border-radius:9px",
    "display:flex",
    "align-items:center",
    "justify-content:center",
    `background:color-mix(in oklch, ${meta.color} 92%, white)`,
    `box-shadow:0 8px 22px rgba(2,6,23,.45), 0 0 0 1px color-mix(in oklch, ${meta.color} 60%, transparent)`,
    "will-change:transform,opacity",
  ].join(";");
  fly.appendChild(createFlightIcon(meta.iconPath));
  document.body.appendChild(fly);

  if (typeof fly.animate !== "function") {
    fly.remove();
    pulseTarget(target, meta.color);
    return;
  }

  const lift = Math.min(120, Math.max(48, distance * 0.32));
  const controlX = startX + dx * 0.5;
  const controlY = startY + dy * 0.5 - lift;
  const frames: Keyframe[] = [];
  const frameCount = 26;

  for (let i = 0; i <= frameCount; i += 1) {
    const progress = i / frameCount;
    const inverse = 1 - progress;
    const x =
      inverse * inverse * startX +
      2 * inverse * progress * controlX +
      progress * progress * endX;
    const y =
      inverse * inverse * startY +
      2 * inverse * progress * controlY +
      progress * progress * endY;
    const scale = 1 - 0.66 * progress;
    const opacity = progress < 0.82 ? 1 : 1 - (progress - 0.82) / 0.18;
    const rotation = meta.rotation * progress;

    frames.push({
      offset: progress,
      opacity,
      transform: `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px) scale(${scale.toFixed(3)}) rotate(${rotation.toFixed(1)}deg)`,
    });
  }

  const animation = fly.animate(frames, {
    duration: 620,
    easing: "cubic-bezier(.34,.02,.2,1)",
  });
  const finish = () => {
    fly.remove();
    pulseTarget(target, meta.color);
  };
  animation.addEventListener("finish", finish, { once: true });
  animation.addEventListener("cancel", () => fly.remove(), { once: true });
}

export function flyVerdictToGrade(
  originRect: DOMRect | null,
  target: HTMLElement | null | undefined,
  verdict: DatasetJudgmentVerdict,
) {
  flyEvalCueToTarget(originRect, target, verdict);
}

export function flyUndoToReviewQueue(
  originRect: DOMRect | null,
  target: HTMLElement | null | undefined,
) {
  flyEvalCueToTarget(originRect, target, "undo");
}
