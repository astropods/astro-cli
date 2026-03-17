/**
 * Auto-registering entrypoint for the <astro-trading-card> custom element.
 *
 * Usage:
 *   import "astro-trading-card/element";
 *   const card = document.createElement("astro-trading-card");
 *   card.svg = svgString;
 */
export { AstroTradingCardElement } from "./card-element";
import { registerCardElement } from "./card-element";
registerCardElement();
