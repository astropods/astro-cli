/**
 * <astro-trading-card> web component.
 *
 * Encapsulates the card SVG with holographic shine/glare effects
 * and 3D mouse tracking, all inside shadow DOM.
 *
 * Usage:
 *   import "astro-trading-card/element";
 *   const el = document.createElement("astro-trading-card");
 *   el.svg = generatedSvgString;
 *   document.body.appendChild(el);
 */

const CARD_STYLES = /* css */ `
  :host {
    display: inline-block;
    perspective: 600px;
  }

  .card {
    position: relative;
    display: inline-block;
    border-radius: 16px;
    transform-style: preserve-3d;
    transform: rotateY(var(--rotate-x, 0deg)) rotateX(var(--rotate-y, 0deg));
    transition: transform 0.15s ease-out, box-shadow 0.15s ease-out;
    will-change: transform;
    user-select: none;
    -webkit-user-select: none;
    box-shadow: 0 8px 32px rgba(0,0,0,0.5), 0 2px 8px rgba(0,0,0,0.3);
  }

  .card:hover {
    box-shadow: 0 12px 48px rgba(0,0,0,0.6), 0 4px 12px rgba(0,0,0,0.4);
  }

  .card svg {
    display: block;
    border-radius: 16px;
  }

  .card__shine {
    position: absolute;
    inset: 0;
    border-radius: 16px;
    z-index: 2;
    pointer-events: none;
    mix-blend-mode: color-dodge;
    transition: opacity 0.15s ease-out;
    opacity: var(--shine-idle, 0.4);

    background-image:
      radial-gradient(
        circle at var(--pointer-x, 50%) var(--pointer-y, 50%),
        #fff 5%, #000 50%, #fff 80%
      ),
      linear-gradient(
        -45deg,
        #000 15%, #fff, #000 85%
      ),
      repeating-linear-gradient(
        var(--foil-angle, 135deg),
        hsl(280, 80%, 50%) 0%,
        hsl(200, 80%, 50%) 10%,
        hsl(140, 80%, 50%) 20%,
        hsl(60, 80%, 50%) 30%,
        hsl(330, 80%, 50%) 40%,
        hsl(280, 80%, 50%) 50%
      );

    background-blend-mode: soft-light, difference;
    background-size: 120% 120%, 200% 200%, 150% 150%;
    background-position:
      center center,
      calc(100% * var(--pointer-from-left, 0.5)) calc(100% * var(--pointer-from-top, 0.5)),
      center center;

    filter: brightness(0.55) contrast(1.5) saturate(1);
  }

  :host(:hover) .card__shine {
    opacity: calc(1.5 * var(--card-opacity, 0.4) - var(--pointer-from-center, 0));
  }

  .card__glare {
    position: absolute;
    inset: 0;
    border-radius: 16px;
    z-index: 3;
    pointer-events: none;
    mix-blend-mode: overlay;
    transition: opacity 0.15s ease-out;
    opacity: var(--glare-idle, 0.3);

    background-image:
      radial-gradient(
        farthest-corner circle at var(--pointer-x, 50%) var(--pointer-y, 50%),
        hsla(0, 0%, 100%, 0.8) 10%,
        hsla(0, 0%, 100%, 0.5) 20%,
        hsla(0, 0%, 0%, 0.75) 90%
      );
    filter: brightness(0.7) contrast(1.5);
  }

  :host(:hover) .card__glare {
    opacity: var(--card-opacity, 0.3);
  }
`;

function clamp(v: number, min = 0, max = 100) {
  return Math.min(max, Math.max(min, v));
}

export class AstroTradingCardElement extends HTMLElement {
  private _shadow: ShadowRoot;
  private _card: HTMLDivElement;
  private _svgSlot: HTMLDivElement;

  constructor() {
    super();
    this._shadow = this.attachShadow({ mode: "open" });

    const style = document.createElement("style");
    style.textContent = CARD_STYLES;
    this._shadow.appendChild(style);

    this._card = document.createElement("div");
    this._card.className = "card";

    this._svgSlot = document.createElement("div");
    this._svgSlot.className = "card__svg";

    const shine = document.createElement("div");
    shine.className = "card__shine";

    const glare = document.createElement("div");
    glare.className = "card__glare";

    this._card.appendChild(this._svgSlot);
    this._card.appendChild(shine);
    this._card.appendChild(glare);
    this._shadow.appendChild(this._card);

    this._onPointerMove = this._onPointerMove.bind(this);
    this._onPointerLeave = this._onPointerLeave.bind(this);
  }

  connectedCallback() {
    this._card.addEventListener("pointermove", this._onPointerMove);
    this._card.addEventListener("pointerleave", this._onPointerLeave);
  }

  disconnectedCallback() {
    this._card.removeEventListener("pointermove", this._onPointerMove);
    this._card.removeEventListener("pointerleave", this._onPointerLeave);
  }

  /** Set the card SVG content. */
  set svg(value: string) {
    this._svgSlot.innerHTML = value;
  }

  get svg(): string {
    return this._svgSlot.innerHTML;
  }

  /** Idle shine opacity (0-1, default 0.4). */
  set shineIdle(value: number) {
    this._card.style.setProperty("--shine-idle", String(value));
  }

  /** Idle glare opacity (0-1, default 0.3). */
  set glareIdle(value: number) {
    this._card.style.setProperty("--glare-idle", String(value));
  }

  private _onPointerMove(e: PointerEvent) {
    const rect = this._card.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;
    const px = clamp((x / rect.width) * 100);
    const py = clamp((y / rect.height) * 100);
    const cx = px - 50;
    const cy = py - 50;
    const dist = Math.sqrt(cx * cx + cy * cy) / 50;

    const s = this._card.style;
    s.setProperty("--pointer-x", `${px}%`);
    s.setProperty("--pointer-y", `${py}%`);
    s.setProperty("--pointer-from-left", String(px / 100));
    s.setProperty("--pointer-from-top", String(py / 100));
    s.setProperty("--pointer-from-center", String(clamp(dist, 0, 1)));
    s.setProperty("--card-opacity", "1");
    s.setProperty("--rotate-x", `${-(cx / 4)}deg`);
    s.setProperty("--rotate-y", `${cy / 4}deg`);
  }

  private _onPointerLeave() {
    const s = this._card.style;
    s.setProperty("--card-opacity", "0");
    s.setProperty("--rotate-x", "0deg");
    s.setProperty("--rotate-y", "0deg");
  }
}

/** Register the <astro-trading-card> custom element. */
export function registerCardElement(
  tagName = "astro-trading-card",
) {
  if (!customElements.get(tagName)) {
    customElements.define(tagName, AstroTradingCardElement);
  }
}
