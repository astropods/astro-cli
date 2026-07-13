import type { CardAccount, CardAvatar, CardIntegration, CardStat, ResolvedCardColors } from "./types";

export interface AvatarRenderOptions {
  avatar: CardAvatar | undefined;
  x: number;
  y: number;
  size: number;
  clipId: string;
  borderColor?: string;
  borderWidth?: number;
  radius?: number;
}

/**
 * Render an avatar into SVG markup at the given position and size.
 * Returns an empty string if no avatar is provided.
 */
export function renderAvatar(opts: AvatarRenderOptions): { defs: string; content: string } | null {
  const { avatar, x, y, size, clipId, borderColor, borderWidth = 3, radius } = opts;
  if (!avatar) return null;

  const r = radius ?? Math.round(size * 0.1);

  const clip = `<clipPath id="${clipId}"><rect x="${x}" y="${y}" width="${size}" height="${size}" rx="${r}"/></clipPath>`;

  let content: string;
  if (avatar.svg) {
    content = `<g clip-path="url(#${clipId})"><g transform="translate(${x}, ${y})"><g transform="scale(${size / 128})">${avatar.svg}</g></g></g>`;
  } else {
    content = `<g clip-path="url(#${clipId})"><image href="${escapeXml(avatar.url!)}" x="${x}" y="${y}" width="${size}" height="${size}" preserveAspectRatio="xMidYMid slice"/></g>`;
  }

  const border = borderColor
    ? `<rect x="${x}" y="${y}" width="${size}" height="${size}" rx="${r}" fill="none" stroke="${borderColor}" stroke-width="${borderWidth}" opacity="0.3"/>`
    : "";

  return { defs: clip, content: `${content}\n    ${border}` };
}

/** Strip the outer `<svg>` wrapper, returning only the inner content. */
export function stripSvgWrapper(svg: string): string {
  return svg.replace(/<svg[^>]*>/, "").replace(/<\/svg>/, "");
}

/** Escape text for safe SVG embedding. */
export function escapeXml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

/**
 * Wraps text to fit within a given width, approximating character count.
 * Returns an array of lines.
 */
export function wrapText(
  text: string,
  maxCharsPerLine: number,
  maxLines: number,
): string[] {
  const words = text.split(/\s+/);
  const lines: string[] = [];
  let current = "";

  for (const word of words) {
    if (lines.length >= maxLines) break;
    const candidate = current ? `${current} ${word}` : word;
    if (candidate.length > maxCharsPerLine && current) {
      lines.push(current);
      current = word;
    } else {
      current = candidate;
    }
  }

  if (current && lines.length < maxLines) {
    lines.push(current);
  }

  // Truncate last line if we ran out of space
  if (lines.length === maxLines && words.length > 0) {
    const last = lines[maxLines - 1];
    if (last.length > maxCharsPerLine) {
      lines[maxLines - 1] = last.slice(0, maxCharsPerLine - 1) + "\u2026";
    }
  }

  return lines;
}

export interface StatRowsOptions {
  stats: CardStat[];
  x: number;
  y: number;
  width: number;
  colors: ResolvedCardColors;
  rowHeight?: number;
  padding?: number;
  maxWrappedLines?: number;
}

/** Get initials from a full name (up to 2 chars). */
function getInitials(fullName: string): string {
  const parts = fullName.trim().split(/\s+/);
  if (parts.length === 1) return parts[0][0]?.toUpperCase() ?? "";
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

/** Measure text width using a canvas context if available, otherwise approximate. */
function measureTextWidth(text: string, font: string, fallbackCharWidth: number): number {
  if (typeof document !== "undefined") {
    const canvas = document.createElement("canvas");
    const ctx = canvas.getContext("2d");
    if (ctx) {
      ctx.font = font;
      return ctx.measureText(text).width;
    }
  }
  return text.length * fallbackCharWidth;
}

/** Truncate text to fit within maxWidth, appending an ellipsis if needed. */
function truncateText(text: string, font: string, maxWidth: number, fallbackCharWidth: number): string {
  if (measureTextWidth(text, font, fallbackCharWidth) <= maxWidth) return text;
  const ellipsis = "\u2026";
  for (let len = text.length - 1; len > 0; len--) {
    const candidate = text.slice(0, len) + ellipsis;
    if (measureTextWidth(candidate, font, fallbackCharWidth) <= maxWidth) return candidate;
  }
  return ellipsis;
}

export function findNaturalTextBreak(text: string | string[], maxChars: number, minBreak?: number): number {
  const chars = Array.isArray(text) ? text : Array.from(text);
  const minUsefulBreak = Math.max(minBreak ?? 1, Math.ceil(maxChars * 0.55));
  for (let i = Math.min(maxChars, chars.length); i >= minUsefulBreak; i--) {
    const prev = chars[i - 1] ?? "";
    const next = chars[i] ?? "";
    if (/\s/.test(prev) || prev === "/" || prev === "-" || prev === "_" || prev === ".") return i;
    if (/[a-z0-9]/.test(prev) && /[A-Z]/.test(next)) return i;
  }
  return -1;
}

function hyphenateValue(text: string, maxWidth: number, fallbackCharWidth: number, maxLines = Infinity): string[] {
  const normalized = text.trim().replace(/\s+/g, " ");
  if (!normalized) return [""];

  const maxChars = Math.max(8, Math.floor(maxWidth / fallbackCharWidth));
  const lines: string[] = [];
  let remaining = normalized;

  while (remaining.length > maxChars && lines.length < maxLines - 1) {
    const naturalBreak = findNaturalTextBreak(remaining, maxChars);
    if (naturalBreak > 0) {
      lines.push(remaining.slice(0, naturalBreak).trimEnd());
      remaining = remaining.slice(naturalBreak).trimStart();
      continue;
    }

    const hardBreak = Math.max(1, maxChars - 1);
    lines.push(`${remaining.slice(0, hardBreak)}-`);
    remaining = remaining.slice(hardBreak);
  }

  if (remaining && lines.length < maxLines) {
    if (remaining.length <= maxChars) {
      lines.push(remaining);
    } else {
      lines.push(`${remaining.slice(0, Math.max(1, maxChars - 1)).trimEnd()}\u2026`);
    }
  }
  return lines;
}

function renderHyphenatedValue(
  lines: string[],
  rightX: number,
  firstLineY: number,
  lineHeight: number,
  colors: ResolvedCardColors,
  clipId: string,
): string {
  const tspans = lines
    .map(
      (line, index) =>
        `<tspan class="stat-value-line" x="${rightX}" dy="${index === 0 ? 0 : lineHeight}">${escapeXml(line)}</tspan>`,
    )
    .join("");
  return `<text x="${rightX}" y="${firstLineY}" font-family="system-ui, sans-serif" font-size="10" font-weight="700" fill="${colors.foreground}" text-anchor="end" clip-path="url(#${clipId})">${tspans}</text>`;
}

/** Render an account value: small avatar circle + handle, right-aligned. */
function renderAccountValue(
  account: CardAccount,
  rightX: number,
  textY: number,
  colors: ResolvedCardColors,
  clipId: string,
  maxWidth: number,
): string {
  const avatarSize = 16;
  const gap = 5;
  const handleFont = "700 13px system-ui, sans-serif";
  const fallbackCharWidth = 13 * 0.55;
  // Reserve space for avatar + gap, then truncate the handle to fit
  const maxHandleWidth = maxWidth - avatarSize - gap;
  const handleText = truncateText(account.handle, handleFont, maxHandleWidth, fallbackCharWidth);
  const handleWidth = measureTextWidth(handleText, handleFont, fallbackCharWidth);
  const avatarX = rightX - handleWidth - gap - avatarSize;
  const avatarCenterX = avatarX + avatarSize / 2;
  const avatarCenterY = textY - avatarSize * 0.3;
  const avatarR = avatarSize / 2;

  let avatarEl: string;
  if (account.avatarUrl) {
    avatarEl = [
      `<clipPath id="${clipId}"><circle cx="${avatarCenterX}" cy="${avatarCenterY}" r="${avatarR}"/></clipPath>`,
      `<image href="${escapeXml(account.avatarUrl)}" x="${avatarCenterX - avatarR}" y="${avatarCenterY - avatarR}" width="${avatarSize}" height="${avatarSize}" clip-path="url(#${clipId})" preserveAspectRatio="xMidYMid slice"/>`,
    ].join("\n    ");
  } else {
    const initials = getInitials(account.fullName);
    avatarEl = [
      `<circle cx="${avatarCenterX}" cy="${avatarCenterY}" r="${avatarR}" fill="${colors.accent}"/>`,
      `<text x="${avatarCenterX}" y="${avatarCenterY}" font-family="system-ui, sans-serif" font-size="7" font-weight="600" fill="${colors.foreground}" text-anchor="middle" dominant-baseline="central">${escapeXml(initials)}</text>`,
    ].join("\n    ");
  }

  const handleEl = `<text x="${rightX}" y="${textY}" font-family="system-ui, sans-serif" font-size="13" font-weight="700" fill="${colors.foreground}" text-anchor="end">${escapeXml(handleText)}</text>`;

  return `${avatarEl}\n    ${handleEl}`;
}

/**
 * Render a list of label-value stat rows.
 * Returns SVG markup and the total height consumed.
 */
export function renderStatRows(opts: StatRowsOptions): { content: string; height: number } {
  const { stats, x, y, width, colors, rowHeight = 32, padding = 20, maxWrappedLines } = opts;
  if (stats.length === 0) return { content: "", height: 0 };

  let currentY = y;
  const lines = stats.map((stat, i) => {
    const rowY = currentY;

    const labelWidth = measureTextWidth(stat.label.toUpperCase(), "300 9px ui-monospace, monospace", 9 * 0.65);
    const labelGap = 12;
    const valueX = x + padding + labelWidth + labelGap;
    const valueRightX = x + width - padding;
    const availableWidth = Math.max(8, valueRightX - valueX);
    const valueClipId = `stat-value-clip-${i}`;
    const shouldWrapValue = !stat.account && stat.wrap === true;
    const hyphenatedLines = shouldWrapValue ? hyphenateValue(stat.value!, availableWidth, 10 * 0.55, maxWrappedLines) : [];
    const valueLineHeight = 11;
    const currentRowHeight = shouldWrapValue
      ? Math.max(rowHeight, 18 + (hyphenatedLines.length - 1) * valueLineHeight + 12)
      : rowHeight;
    const labelY = rowY + Math.min(rowHeight * 0.6, currentRowHeight * 0.45);
    const dividerY = rowY + currentRowHeight;
    const valueClip = `<clipPath id="${valueClipId}"><rect x="${valueX}" y="${rowY}" width="${availableWidth}" height="${currentRowHeight}"/></clipPath>`;
    const labelEl = `<text x="${x + padding}" y="${labelY}" font-family="ui-monospace, monospace" font-size="9" font-weight="300" fill="${colors.glow}" opacity="0.7" letter-spacing="0.18em">${escapeXml(stat.label.toUpperCase())}</text>`;

    let valueEl: string;
    if (stat.account) {
      valueEl = renderAccountValue(stat.account, valueRightX, labelY + 2, colors, `stat-avatar-${i}`, availableWidth);
    } else if (shouldWrapValue) {
      valueEl = renderHyphenatedValue(hyphenatedLines, valueRightX, rowY + 18, valueLineHeight, colors, valueClipId);
    } else {
      const valueText = truncateText(stat.value!, "700 13px system-ui, sans-serif", availableWidth, 13 * 0.55);
      valueEl = `<text x="${valueRightX}" y="${labelY + 2}" font-family="system-ui, sans-serif" font-size="13" font-weight="700" fill="${colors.foreground}" text-anchor="end" clip-path="url(#${valueClipId})">${escapeXml(valueText)}</text>`;
    }

    currentY += currentRowHeight;

    return [
      valueClip,
      labelEl,
      valueEl,
      i < stats.length - 1
        ? `<line x1="${x}" y1="${dividerY}" x2="${x + width}" y2="${dividerY}" stroke="${colors.glow}" stroke-width="1" opacity="0.15"/>`
        : "",
    ].join("\n    ");
  });

  return {
    content: lines.join("\n    "),
    height: currentY - y,
  };
}

export interface IntegrationPillsOptions {
  integrations: CardIntegration[];
  x: number;
  y: number;
  width: number;
  colors: ResolvedCardColors;
  padding?: number;
  pillHeight?: number;
  pillGap?: number;
  rowGap?: number;
  iconSize?: number;
  maxRows?: number;
}

/**
 * Approximate the width of a text string at a given font size.
 * Rough heuristic: average char width ≈ fontSize * 0.55 for sans-serif.
 */
function approxTextWidth(text: string, fontSize: number): number {
  return text.length * fontSize * 0.55;
}

/**
 * Render integration pills that wrap into rows.
 * Returns SVG markup and total height consumed.
 */
export function renderIntegrationPills(opts: IntegrationPillsOptions): { content: string; height: number } {
  const {
    integrations, x, y, width, colors,
    padding = 20, pillHeight = 24, pillGap = 6, rowGap = 6, iconSize = 12,
    maxRows = 2,
  } = opts;
  if (integrations.length === 0 || maxRows <= 0) return { content: "", height: 0 };

  const fontSize = 10;
  const pillPadX = 8;
  const iconTextGap = 4;
  const maxWidth = width - padding * 2;

  // Layout pills into rows
  const rows: { integration: CardIntegration; pillWidth: number; x: number; y: number }[] = [];
  let curX = 0;
  let curRow = 0;

  let overflowCount = 0;

  for (let idx = 0; idx < integrations.length; idx++) {
    const integration = integrations[idx];
    const textW = approxTextWidth(integration.name, fontSize);
    const hasIcon = !!(integration.icon || integration.iconUrl);
    const pillW = pillPadX + (hasIcon ? iconSize + iconTextGap : 0) + textW + pillPadX;

    if (curX > 0 && curX + pillW > maxWidth) {
      curRow++;
      curX = 0;
    }

    if (curRow >= maxRows) {
      overflowCount = integrations.length - idx;
      break;
    }

    rows.push({
      integration,
      pillWidth: pillW,
      x: x + padding + curX,
      y: y + curRow * (pillHeight + rowGap),
    });

    curX += pillW + pillGap;
  }

  const totalRows = Math.min(curRow + 1, maxRows);
  const totalHeight = totalRows * pillHeight + (totalRows - 1) * rowGap;

  const pills = rows.map((pill) => {
    const ry = 4;
    const hasIcon = !!(pill.integration.icon || pill.integration.iconUrl);
    const textX = pill.x + pillPadX + (hasIcon ? iconSize + iconTextGap : 0);
    const textY = pill.y + pillHeight / 2 + fontSize * 0.35;

    let iconEl = "";
    if (hasIcon) {
      const iconX = pill.x + pillPadX;
      const iconY = pill.y + (pillHeight - iconSize) / 2;
      const glowColor = colors.glow;
      if (pill.integration.icon) {
        const iconContent = pill.integration.icon.replace(/currentColor/g, glowColor);
        iconEl = `<g transform="translate(${iconX}, ${iconY})" fill="${glowColor}" opacity="0.8"><g transform="scale(${iconSize / 24})">${iconContent}</g></g>`;
      } else {
        iconEl = `<image href="${escapeXml(pill.integration.iconUrl!)}" x="${iconX}" y="${iconY}" width="${iconSize}" height="${iconSize}"/>`;
      }
    }

    return [
      `<rect x="${pill.x}" y="${pill.y}" width="${pill.pillWidth}" height="${pillHeight}" rx="${ry}" fill="${colors.glow}" opacity="0.1"/>`,
      iconEl,
      `<text x="${textX}" y="${textY}" font-family="system-ui, sans-serif" font-size="${fontSize}" fill="${colors.glow}" opacity="0.8">${escapeXml(pill.integration.name)}</text>`,
    ].join("\n    ");
  });

  let overflowEl = "";
  if (overflowCount > 0) {
    const lastPill = rows[rows.length - 1];
    const moreX = lastPill.x + lastPill.pillWidth + pillGap;
    const moreY = lastPill.y + pillHeight / 2 + fontSize * 0.35;
    overflowEl = `\n    <text x="${moreX}" y="${moreY}" font-family="system-ui, sans-serif" font-size="${fontSize}" fill="${colors.glow}" opacity="0.5">+${overflowCount}</text>`;
  }

  return {
    content: pills.join("\n    ") + overflowEl,
    height: totalHeight,
  };
}
