/**
 * Holographic card hover effect — shared pointer math and CSS.
 *
 * Sets CSS custom properties on the card element to drive the
 * shine/glare layers defined in holo.css.
 */

function clamp(v: number, min = 0, max = 100) {
  return Math.min(max, Math.max(min, v));
}

/** Compute CSS custom properties from a pointer event relative to the card. */
export function computeHoloVars(
  rect: DOMRect,
  clientX: number,
  clientY: number,
): Record<string, string> {
  const px = clamp(((clientX - rect.left) / rect.width) * 100);
  const py = clamp(((clientY - rect.top) / rect.height) * 100);
  const cx = px - 50;
  const cy = py - 50;
  const dist = Math.sqrt(cx * cx + cy * cy) / 50;

  return {
    "--px": `${px}%`,
    "--py": `${py}%`,
    "--fl": String(px / 100),
    "--ft": String(py / 100),
    "--fc": String(clamp(dist, 0, 1)),
    "--o": "1",
    "--rx": `${-(cx / 4)}deg`,
    "--ry": `${cy / 4}deg`,
  };
}

/** Reset CSS custom properties when the pointer leaves the card. */
export const HOLO_RESET_VARS: Record<string, string> = {
  "--px": "50%",
  "--py": "50%",
  "--fl": "0.5",
  "--ft": "0.5",
  "--fc": "0",
  "--o": "0",
  "--rx": "0deg",
  "--ry": "0deg",
};

/** Apply holo pointer tracking to a vanilla DOM element. */
export function setupHolo(el: HTMLElement) {
  el.addEventListener("pointermove", (e: PointerEvent) => {
    const rect = el.getBoundingClientRect();
    const vars = computeHoloVars(rect, e.clientX, e.clientY);
    for (const [k, v] of Object.entries(vars)) {
      el.style.setProperty(k, v);
    }
  });
  el.addEventListener("pointerleave", () => {
    for (const [k, v] of Object.entries(HOLO_RESET_VARS)) {
      el.style.setProperty(k, v);
    }
  });
}
