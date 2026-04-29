/**
 * Blueprint badge SVG builder.
 *
 * Produces a 1200×628 social-unfurl card. Layout matches the design-system
 * spec: 12px accent banner at top, 8px dot-grid + radial teal gradient on the
 * left, agent avatar (208×208) left-aligned, name / description text column to
 * the right, Astro AI wordmark (white) bottom-left, circular account avatar +
 * @handle bottom-right.
 */

// ─── Canvas & layout constants ─────────────────────────────────────────────────

export const CANVAS_W  = 1200;
export const CANVAS_H  = 628;

const BANNER_H   = 12;
const PAD_X      = 72;
const PAD_TOP    = 64;
const PAD_BOTTOM = 60;

// Avatar (agent)
const AV_SIZE = 208;
const AV_R    = 22;
const AV_GAP  = 52;   // gap between avatar right edge and text

// Text column
const TEXT_X  = PAD_X + AV_SIZE + AV_GAP;          // 332
const TEXT_W  = CANVAS_W - TEXT_X - PAD_X;          // 796

// Name typography
const NAME_SIZE_MAX = 72;
const NAME_SIZE_MIN = 36;
const NAME_LH_RATIO = 1.04;   // tight line-height (54/52)

// Description typography
const DESC_SIZE = 24;
const DESC_LH   = 36;

// Account avatar (bottom-right circle)
const ACCT_AV_SIZE = 44;

// Astro AI wordmark
// Native viewBox: 0 0 96 17.1, letterform baseline at y≈14.26
const WORDMARK_NATIVE_BASELINE = 14.26;
const WORDMARK_HEIGHT          = 22;   // rendered cap height in px
const WORDMARK_SCALE           = WORDMARK_HEIGHT / WORDMARK_NATIVE_BASELINE;

// ─── Helpers ───────────────────────────────────────────────────────────────────

export interface CardColors {
  background: string;
  foreground: string;
  accent:     string;
  glow:       string;
}

function escapeXml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

function wrapText(text: string, maxWidth: number, fontSize: number, maxLines: number): string[] {
  const maxChars = Math.max(1, Math.floor(maxWidth / (fontSize * 0.52)));
  const words = text.split(/\s+/);
  const lines: string[] = [];
  let current = "";
  for (const word of words) {
    if (lines.length >= maxLines) break;
    const candidate = current ? `${current} ${word}` : word;
    if (candidate.length > maxChars && current) {
      lines.push(current);
      current = word;
    } else {
      current = candidate;
    }
  }
  if (current && lines.length < maxLines) lines.push(current);
  if (lines.length === maxLines) {
    const last = lines[maxLines - 1];
    if (last.length > maxChars) lines[maxLines - 1] = last.slice(0, maxChars - 1) + "\u2026";
  }
  return lines;
}

// ─── Input ─────────────────────────────────────────────────────────────────────

export interface BlueprintCardInput {
  name:              string;
  account:           string;
  description:       string;
  agentAvatarUri:    string | null;
  accountAvatarUri:  string | null;
}

// ─── SVG builder ───────────────────────────────────────────────────────────────

export function buildBlueprintBadgeSvg(input: BlueprintCardInput, colors: CardColors): string {
  const { name, account, description, agentAvatarUri, accountAvatarUri } = input;

  // ── Name font size (scales down for long names) ──────────────────────────────
  const nameFontSize = Math.round(
    Math.min(NAME_SIZE_MAX, Math.max(NAME_SIZE_MIN, (TEXT_W * 0.85) / (name.length * 0.54))),
  );
  const nameLH = Math.round(nameFontSize * NAME_LH_RATIO);

  // ── Description lines (needed for text-block height) ─────────────────────────
  // Wrap at the name's rendered right edge so description never exceeds the name width.
  const nameRenderedW = Math.round(nameFontSize * name.length * 0.54);
  const descLines = description ? wrapText(description, nameRenderedW, DESC_SIZE, 3) : [];

  // ── Vertical layout ───────────────────────────────────────────────────────────
  // Content area: y = BANNER_H + PAD_TOP → CANVAS_H - PAD_BOTTOM
  const contentAreaTop    = BANNER_H + PAD_TOP;
  const contentAreaBottom = CANVAS_H - PAD_BOTTOM;
  const contentAreaH      = contentAreaBottom - contentAreaTop;   // 504

  // Bottom bar height: account avatar or handle text
  const bottomBarH = ACCT_AV_SIZE;

  // Main block height: avatar row (AV_SIZE) + 64px gap + bottomBar
  const mainBlockH = AV_SIZE + 64 + bottomBarH;

  // Vertically centre the main block in the content area
  const mainBlockTop  = contentAreaTop + Math.round((contentAreaH - mainBlockH) / 2);
  const avatarY       = mainBlockTop;
  const bottomBarY    = mainBlockTop + AV_SIZE + 64;

  // Vertically centre the name+description block on the avatar.
  // textBlockH = name line-height + gap + desc lines (treat as top-of-cap to bottom-of-last-line)
  const textBlockH = nameLH + (descLines.length > 0 ? 14 + descLines.length * DESC_LH : 0);
  const avatarCenterY  = avatarY + AV_SIZE / 2;
  const nameCapTop     = Math.round(avatarCenterY - textBlockH / 2);

  // Name baseline (cap-height fraction ~0.75 of font-size)
  const nameBaselineY = nameCapTop + Math.round(nameFontSize * 0.75);

  // Description first-line baseline
  const descStartY = nameCapTop + nameLH + 14 + Math.round(DESC_SIZE * 0.75);

  // ── Agent avatar ──────────────────────────────────────────────────────────────
  const agentAvatarEl = agentAvatarUri
    ? `<image href="${escapeXml(agentAvatarUri)}"
        x="${PAD_X}" y="${avatarY}"
        width="${AV_SIZE}" height="${AV_SIZE}"
        clip-path="url(#agentAvatarClip)" preserveAspectRatio="xMidYMid slice"/>`
    : `<rect x="${PAD_X}" y="${avatarY}" width="${AV_SIZE}" height="${AV_SIZE}"
        rx="${AV_R}" fill="${colors.accent}" opacity="0.35"/>`;

  // ── Account avatar (bottom-right circle) ─────────────────────────────────────
  // Bottom-right: [avatar] [24px gap] [@handle]
  // Anchor the group to the right edge; estimate handle width from account length.
  const handleWidth    = Math.round(DESC_SIZE * 0.62 * (account.length + 1)); // +1 for "@"
  const acctAvatarCX   = CANVAS_W - PAD_X - handleWidth - 24 - ACCT_AV_SIZE / 2;
  const acctAvatarCY   = bottomBarY + ACCT_AV_SIZE / 2;
  const handleX        = CANVAS_W - PAD_X;
  const handleBaselineY = acctAvatarCY + Math.round(DESC_SIZE * 0.35);

  const accountAvatarEl = accountAvatarUri
    ? `<image href="${escapeXml(accountAvatarUri)}"
        x="${acctAvatarCX - ACCT_AV_SIZE / 2}" y="${acctAvatarCY - ACCT_AV_SIZE / 2}"
        width="${ACCT_AV_SIZE}" height="${ACCT_AV_SIZE}"
        clip-path="url(#acctAvatarClip)" preserveAspectRatio="xMidYMid slice"/>`
    : `<circle cx="${acctAvatarCX}" cy="${acctAvatarCY}" r="${ACCT_AV_SIZE / 2}"
        fill="${colors.accent}" opacity="0.4"/>
       <text x="${acctAvatarCX}" y="${acctAvatarCY + 8}" text-anchor="middle"
         font-family="Geist, Inter, sans-serif" font-size="18" font-weight="700"
         fill="#ffffff">${escapeXml(account.slice(0, 2).toUpperCase())}</text>`;

  // ── Wordmark (Astro AI) ───────────────────────────────────────────────────────
  const wmX = PAD_X;
  const wmY = bottomBarY + Math.round((ACCT_AV_SIZE - WORDMARK_HEIGHT) / 2);

  // ── Description SVG ───────────────────────────────────────────────────────────
  const descSvg = descLines.map((line, i) =>
    `<text x="${TEXT_X}" y="${descStartY + i * DESC_LH}"
      font-family="Geist, Inter, sans-serif" font-size="${DESC_SIZE}" font-weight="400"
      fill="#ffffff" opacity="0.65">${escapeXml(line)}</text>`
  ).join("\n  ");

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${CANVAS_W}" height="${CANVAS_H}"
  viewBox="0 0 ${CANVAS_W} ${CANVAS_H}">
  <defs>
    <!-- Agent avatar clip -->
    <clipPath id="agentAvatarClip">
      <rect x="${PAD_X}" y="${avatarY}" width="${AV_SIZE}" height="${AV_SIZE}" rx="${AV_R}"/>
    </clipPath>
    <!-- Account avatar clip (circle) -->
    <clipPath id="acctAvatarClip">
      <circle cx="${acctAvatarCX}" cy="${acctAvatarCY}" r="${ACCT_AV_SIZE / 2}"/>
    </clipPath>
    <!-- Background mask: radial fade from top-left to transparent -->
    <mask id="bgMask">
      <radialGradient id="bgMaskGrad" gradientUnits="userSpaceOnUse"
        cx="${CANVAS_W * 0.30}" cy="0" r="${CANVAS_W * 1.10}"
        fx="${CANVAS_W * 0.30}" fy="0">
        <stop offset="0%"  stop-color="#ffffff"/>
        <stop offset="80%" stop-color="#000000"/>
      </radialGradient>
      <rect width="${CANVAS_W}" height="${CANVAS_H}" fill="url(#bgMaskGrad)"/>
    </mask>
    <!-- Teal radial gradient: accent color blooming from top-left -->
    <radialGradient id="tealGrad" gradientUnits="userSpaceOnUse"
      cx="${CANVAS_W * 0.20}" cy="0" r="${CANVAS_W * 0.90}"
      fx="${CANVAS_W * 0.20}" fy="0">
      <stop offset="0%"   stop-color="${colors.accent}" stop-opacity="0.55"/>
      <stop offset="75%"  stop-color="${colors.accent}" stop-opacity="0"/>
    </radialGradient>
    <!-- 8×8 dot-grid pattern (design-system spec) -->
    <pattern id="dotGrid" width="8" height="8" patternUnits="userSpaceOnUse">
      <path d="M 8 0 L 0 0 0 8" fill="none" stroke="#ffffff" stroke-width="0.5" stroke-opacity="0.12"/>
    </pattern>
  </defs>

  <!-- Base background -->
  <rect width="${CANVAS_W}" height="${CANVAS_H}" fill="${colors.background}"/>

  <!-- Gradient + grid masked to top-left bloom -->
  <g mask="url(#bgMask)">
    <rect width="${CANVAS_W}" height="${CANVAS_H}" fill="url(#tealGrad)"/>
    <rect width="${CANVAS_W}" height="${CANVAS_H}" fill="url(#dotGrid)"/>
  </g>

  <!-- Top accent banner -->
  <rect width="${CANVAS_W}" height="${BANNER_H}" fill="${colors.accent}"/>

  <!-- Agent avatar -->
  ${agentAvatarEl}
  <rect x="${PAD_X}" y="${avatarY}" width="${AV_SIZE}" height="${AV_SIZE}"
    rx="${AV_R}" fill="none" stroke="#ffffff" stroke-width="1" opacity="0.10"/>

  <!-- Agent name -->
  <text x="${TEXT_X}" y="${nameBaselineY}"
    font-family="Geist, Inter, sans-serif"
    font-size="${nameFontSize}" font-weight="700"
    letter-spacing="-0.025em"
    fill="#ffffff">${escapeXml(name)}</text>

  <!-- Description -->
  ${descSvg}

  <!-- Astro AI wordmark — bottom-left, full white -->
  <g transform="translate(${wmX}, ${wmY}) scale(${WORDMARK_SCALE.toFixed(3)})" fill="#ffffff" opacity="0.9">
    <path d="M1.74591 0.293999C1.79518 0.120669 1.96077 -0.00336212 2.13839 6.94822e-05L18.8849 0.326554C18.9976 0.328749 19.0984 0.382388 19.1581 0.472038C19.2179 0.561691 19.2299 0.677273 19.1908 0.785326L13.38 16.8294C13.3181 17.0001 13.1443 17.1126 12.9683 17.0958C12.7923 17.0789 12.6655 16.9376 12.6676 16.7612L12.7627 8.92474L12.7488 8.9414L12.551 5.75657C12.5186 5.23539 12.206 4.79453 11.7303 4.5987L9.1624 3.54171L9.16698 3.54019L1.93879 0.703143C1.77809 0.640067 1.69667 0.467359 1.74591 0.293999Z"/>
    <path d="M0.00104344 6.87966C-0.0124278 6.70252 0.10582 6.5302 0.281643 6.47063L9.05733 3.4984L11.7303 4.5987C12.206 4.79453 12.5185 5.23529 12.5509 5.75646L12.7525 8.99843L12.7405 9.01937L12.741 8.98175L6.7757 16.1991C6.6564 16.3434 6.45684 16.3915 6.30148 16.3134C6.14614 16.2353 6.07933 16.0532 6.14271 15.8808L8.85164 8.51676C9.07232 7.91686 8.67593 7.32128 8.04945 7.31147L0.336444 7.19068C0.155878 7.18784 0.0145416 7.05679 0.00104344 6.87966Z"/>
    <path d="M25.957 12.7613L29.5076 2.7085H31.8607L35.4308 12.7613H36.3889V14.2569H31.9004V12.7613H33.4764L32.7375 10.5276H28.5292L27.7708 12.7613H29.401V14.2569H24.9785V12.7613H25.9561H25.957ZM32.2991 9.2106L30.6249 4.22441L28.9685 9.2106H32.2999H32.2991Z"/>
    <path d="M41.3348 14.4363C39.8586 14.4363 38.4223 14.1968 37.2449 13.6584V11.5043H38.8201V12.7206C39.4794 13.0202 40.2573 13.1599 41.2738 13.1599C42.849 13.1599 43.6869 12.7012 43.6869 11.8632C43.6869 11.2648 43.2485 10.9652 42.2108 10.806L39.9974 10.5064C38.1235 10.2474 37.2246 9.44925 37.2246 8.0933C37.2246 6.47751 38.8404 5.40088 41.4135 5.40088C42.7288 5.40088 43.9662 5.58116 45.1834 6.0196V7.85038H43.6082V7.03699C43.0301 6.83723 42.2116 6.69758 41.3949 6.69758C39.9577 6.69758 39.1603 7.13602 39.1603 7.8944C39.1603 8.47334 39.5988 8.83222 40.5764 8.97188L42.8905 9.29097C44.7644 9.57029 45.6633 10.3481 45.6633 11.6643C45.6633 13.3393 43.9874 14.4371 41.3348 14.4371V14.4363Z"/>
    <path d="M52.6142 12.9009C53.2914 12.9009 54.0498 12.801 55.5267 12.3025L55.944 13.7186C54.2698 14.317 53.2313 14.4566 52.2943 14.4566C49.9802 14.4566 48.8833 13.4596 48.8833 11.7041V7.07678H45.9302V5.62096H48.8833V3.12744H50.7378V5.62096H55.4658V7.07678H50.7378V11.4248C50.7378 12.3821 51.3159 12.9009 52.6134 12.9009H52.6142Z"/>
    <path d="M62.1491 8.13266C62.8279 7.37513 63.3654 7.11612 64.1839 7.11612C64.4623 7.11612 64.7772 7.17622 65.1953 7.3108C65.4687 6.77079 65.8157 6.27987 66.2432 5.85921C65.5677 5.58328 64.9609 5.41992 64.3828 5.41992C63.246 5.41992 62.4276 5.95824 61.6903 7.21431L60.8118 8.57025V5.61883H56.2251V7.0738H58.9573V12.7989H56.2251V14.2547H63.6845V12.7989H60.8118V9.6071L62.1483 8.13097L62.1491 8.13266Z"/>
    <path d="M65.0259 9.9484C65.0259 7.31522 67.0996 5.44043 69.7928 5.44043C72.4861 5.44043 74.5615 7.31522 74.5615 9.9484C74.5615 12.5816 72.4861 14.4564 69.7928 14.4564C67.0996 14.4564 65.0259 12.5816 65.0259 9.9484ZM72.7451 9.9484C72.7451 8.23273 71.4687 6.99613 69.792 6.99613C68.1153 6.99613 66.8406 8.23273 66.8406 9.9484C66.8406 11.6641 68.117 12.9007 69.792 12.9007C71.467 12.9007 72.7451 11.6641 72.7451 9.9484Z"/>
    <path d="M78.6708 12.7613L82.2215 2.7085H84.5745L88.1447 12.7613H89.1028V14.2569H84.6143V12.7613H86.1903L85.4514 10.5276H81.2431L80.4847 12.7613H82.1149V14.2569H77.6924V12.7613H78.67H78.6708ZM85.013 9.2106L83.3388 4.22441L81.6823 9.2106H85.0138H85.013Z"/>
    <path d="M91.9915 12.7955V4.3416H89.9136V2.86377H96.0009V4.3416H93.9035V12.7955H96.0009V14.2734H89.9136V12.7955H91.9915Z"/>
  </g>

  <!-- Account avatar circle + @handle — bottom-right -->
  ${accountAvatarEl}
  ${accountAvatarUri ? `<circle cx="${acctAvatarCX}" cy="${acctAvatarCY}" r="${ACCT_AV_SIZE / 2}"
    fill="none" stroke="#ffffff" stroke-width="1" opacity="0.15"/>` : ""}
  <text x="${handleX}" y="${handleBaselineY}" text-anchor="end"
    font-family="Geist Mono, monospace" font-size="${DESC_SIZE}"
    letter-spacing="0.02em" fill="#ffffff">@${escapeXml(account)}</text>
</svg>`;
}
