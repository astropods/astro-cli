import type { CardAccount, CardAvatar, CardColors, CardIntegration, CardStat } from "./types";

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
  colors: CardColors;
  rowHeight?: number;
  padding?: number;
}

/** Get initials from a full name (up to 2 chars). */
function getInitials(fullName: string): string {
  const parts = fullName.trim().split(/\s+/);
  if (parts.length === 1) return parts[0][0]?.toUpperCase() ?? "";
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

/** Render an account value: small avatar circle + @handle, right-aligned. */
function renderAccountValue(
  account: CardAccount,
  rightX: number,
  textY: number,
  colors: CardColors,
  clipId: string,
): string {
  const avatarSize = 16;
  const gap = 3;
  const handleText = `@${account.handle}`;
  const handleWidth = handleText.length * 11 * 0.55; // approximate
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

  const handleEl = `<text x="${rightX}" y="${textY}" font-family="system-ui, sans-serif" font-size="11" font-weight="500" fill="${colors.foreground}" text-anchor="end">${escapeXml(handleText)}</text>`;

  return `${avatarEl}\n    ${handleEl}`;
}

/**
 * Render a list of label-value stat rows.
 * Returns SVG markup and the total height consumed.
 */
export function renderStatRows(opts: StatRowsOptions): { content: string; height: number } {
  const { stats, x, y, width, colors, rowHeight = 32, padding = 20 } = opts;
  if (stats.length === 0) return { content: "", height: 0 };

  const lines = stats.map((stat, i) => {
    const rowY = y + i * rowHeight;
    const labelY = rowY + rowHeight * 0.6;
    const dividerY = rowY + rowHeight;

    const labelEl = `<text x="${x + padding}" y="${labelY}" font-family="ui-monospace, monospace" font-size="7" font-weight="300" fill="${colors.glow ?? colors.foreground}" opacity="0.7" letter-spacing="0.18em">${escapeXml(stat.label.toUpperCase())}</text>`;

    let valueEl: string;
    if (stat.account) {
      valueEl = renderAccountValue(stat.account, x + width - padding, labelY + 2, colors, `stat-avatar-${i}`);
    } else {
      valueEl = `<text x="${x + width - padding}" y="${labelY + 2}" font-family="system-ui, sans-serif" font-size="13" font-weight="700" fill="${colors.foreground}" text-anchor="end">${escapeXml(stat.value!.toUpperCase())}</text>`;
    }

    return [
      labelEl,
      valueEl,
      i < stats.length - 1
        ? `<line x1="${x}" y1="${dividerY}" x2="${x + width}" y2="${dividerY}" stroke="${colors.glow ?? colors.foreground}" stroke-width="1" opacity="0.15"/>`
        : "",
    ].join("\n    ");
  });

  return {
    content: lines.join("\n    "),
    height: stats.length * rowHeight,
  };
}

export interface IntegrationPillsOptions {
  integrations: CardIntegration[];
  x: number;
  y: number;
  width: number;
  colors: CardColors;
  padding?: number;
  pillHeight?: number;
  pillGap?: number;
  rowGap?: number;
  iconSize?: number;
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
  } = opts;
  if (integrations.length === 0) return { content: "", height: 0 };

  const fontSize = 10;
  const pillPadX = 8;
  const iconTextGap = 4;
  const maxWidth = width - padding * 2;

  // Layout pills into rows
  const rows: { integration: CardIntegration; pillWidth: number; x: number; y: number }[] = [];
  let curX = 0;
  let curRow = 0;

  for (const integration of integrations) {
    const textW = approxTextWidth(integration.name, fontSize);
    const pillW = pillPadX + (integration.icon ? iconSize + iconTextGap : 0) + textW + pillPadX;

    if (curX > 0 && curX + pillW > maxWidth) {
      curRow++;
      curX = 0;
    }

    rows.push({
      integration,
      pillWidth: pillW,
      x: x + padding + curX,
      y: y + curRow * (pillHeight + rowGap),
    });

    curX += pillW + pillGap;
  }

  const totalRows = curRow + 1;
  const totalHeight = totalRows * pillHeight + (totalRows - 1) * rowGap;

  const pills = rows.map((pill) => {
    const ry = 4;
    const textX = pill.x + pillPadX + (pill.integration.icon ? iconSize + iconTextGap : 0);
    const textY = pill.y + pillHeight / 2 + fontSize * 0.35;

    let iconEl = "";
    if (pill.integration.icon) {
      const iconX = pill.x + pillPadX;
      const iconY = pill.y + (pillHeight - iconSize) / 2;
      const glowColor = colors.glow ?? colors.foreground;
      const iconContent = pill.integration.icon.replace(/currentColor/g, glowColor);
      iconEl = `<g transform="translate(${iconX}, ${iconY})" fill="${glowColor}" opacity="0.8"><g transform="scale(${iconSize / 24})">${iconContent}</g></g>`;
    }

    return [
      `<rect x="${pill.x}" y="${pill.y}" width="${pill.pillWidth}" height="${pillHeight}" rx="${ry}" fill="${colors.glow ?? colors.foreground}" opacity="0.1"/>`,
      iconEl,
      `<text x="${textX}" y="${textY}" font-family="system-ui, sans-serif" font-size="${fontSize}" fill="${colors.glow ?? colors.foreground}" opacity="0.8">${escapeXml(pill.integration.name)}</text>`,
    ].join("\n    ");
  });

  return {
    content: pills.join("\n    "),
    height: totalHeight,
  };
}
