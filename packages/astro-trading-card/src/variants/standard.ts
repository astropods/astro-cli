import { CARD_DIMENSIONS, type ResolvedCardColors, type CardData } from "../types";
import { renderAvatar, renderStatRows, renderIntegrationPills, escapeXml, findNaturalTextBreak } from "../svg";
import { renderBarcode } from "../barcode";

const NAME_FONT_SIZE_MAX = 28;
const NAME_LINE_HEIGHT_RATIO = 1.14;
const NAME_LINE_CHAR_LIMITS = [14, 14, 14] as const;
export const NAME_MAX_CHARS = NAME_LINE_CHAR_LIMITS.reduce((sum, limit) => sum + limit, 0);
const NAME_MAX_WIDTH_PAD = 28;
const METADATA_BARCODE_GAP = 12;
const STATS_INTEGRATIONS_GAP = 12;
const INTEGRATIONS_DIVIDER_GAP = 12;
const INTEGRATION_PILL_HEIGHT = 24;
const INTEGRATION_ROW_GAP = 6;
const INTEGRATION_MAX_ROWS = 2;

function capName(text: string): string {
  const normalized = text.trim().replace(/\s+/g, " ");
  const chars = Array.from(normalized);
  if (chars.length <= NAME_MAX_CHARS) return normalized;
  return chars.slice(0, NAME_MAX_CHARS).join("");
}

function maxIntegrationRowsForHeight(availableHeight: number): number {
  if (availableHeight < INTEGRATION_PILL_HEIGHT) return 0;
  const rows = 1 + Math.floor((availableHeight - INTEGRATION_PILL_HEIGHT) / (INTEGRATION_PILL_HEIGHT + INTEGRATION_ROW_GAP));
  return Math.min(INTEGRATION_MAX_ROWS, rows);
}

function wrapName(text: string): string[] {
  let chars = Array.from(text);
  const lines: string[] = [];

  for (let i = 0; i < NAME_LINE_CHAR_LIMITS.length && chars.length > 0; i++) {
    const limit = NAME_LINE_CHAR_LIMITS[i];
    if (chars.length <= limit) {
      const line = chars.join("").trim();
      if (line) lines.push(line);
      break;
    }

    const remainingCapacity = NAME_LINE_CHAR_LIMITS
      .slice(i + 1)
      .reduce((sum, nextLimit) => sum + nextLimit, 0);
    const minBreak = Math.max(1, chars.length - remainingCapacity);
    const naturalBreak = findNaturalTextBreak(chars, limit, minBreak);
    const breakAt = naturalBreak > 0 ? naturalBreak : limit;
    const line = chars.slice(0, breakAt).join("").trimEnd();
    if (line) lines.push(line);
    if (naturalBreak <= 0 && lines.length > 0) {
      lines[lines.length - 1] = `${lines[lines.length - 1]}-`;
    }
    chars = Array.from(chars.slice(breakAt).join("").trimStart());
  }

  return lines;
}

export function renderStandard(data: CardData, colors: ResolvedCardColors): string {
  const { width, height } = CARD_DIMENSIONS.standard;
  const bannerHeight = Math.round(height * 0.075);
  const r = 16;
  // Deterministic uid from card name to keep SVG output stable
  let h = 0;
  const seed = `${data.account ?? ""}/${data.name}`;
  for (let i = 0; i < seed.length; i++) h = (Math.imul(31, h) + seed.charCodeAt(i)) | 0;
  const uid = (h >>> 0).toString(36);

  const nameText = capName(data.displayName || data.name);
  const nameMaxWidth = width - NAME_MAX_WIDTH_PAD * 2;
  const nameFontSize = NAME_FONT_SIZE_MAX;
  const nameLines = wrapName(nameText);
  const nameLineHeight = Math.round(nameFontSize * NAME_LINE_HEIGHT_RATIO);

  const avatarSize = 120;
  const avatarX = (width - avatarSize) / 2;
  const avatarY = bannerHeight + 20;
  const nameGap = nameFontSize + 16;
  const nameY = avatarY + avatarSize + nameGap;
  const nameLastLineY = nameY + (nameLines.length - 1) * nameLineHeight;
  const nameDividerY = nameLastLineY + Math.round(nameFontSize * 0.5) + 14;
  const statsY = nameDividerY;

  const barcodeBottomPadding = 16;
  const barcodeBarHeight = 40;
  const barcodeHeight = (data.barcodeId ? barcodeBarHeight + 20 : 0);
  const barcodeY = height - barcodeHeight - barcodeBottomPadding;
  const metadataBottomY = data.barcodeId ? barcodeY - METADATA_BARCODE_GAP : height - barcodeBottomPadding;
  const barcode = renderBarcode({ id: data.barcodeId ?? "", x: 0, y: barcodeY, width, colors, barHeight: barcodeBarHeight, qrUrl: data.qrUrl });

  let stats = renderStatRows({
    stats: data.stats ?? [],
    x: 0,
    y: statsY,
    width,
    colors,
  });
  let statsDividerY = statsY + stats.height;
  if (statsDividerY > metadataBottomY) {
    for (const maxWrappedLines of [4, 3, 2, 1]) {
      const candidate = renderStatRows({
        stats: data.stats ?? [],
        x: 0,
        y: statsY,
        width,
        colors,
        maxWrappedLines,
      });
      stats = candidate;
      statsDividerY = statsY + stats.height;
      if (statsDividerY <= metadataBottomY) break;
    }
  }

  const integrationsY = statsDividerY + STATS_INTEGRATIONS_GAP;
  const availableIntegrationHeight = metadataBottomY - integrationsY - INTEGRATIONS_DIVIDER_GAP;
  const maxIntegrationRows = maxIntegrationRowsForHeight(availableIntegrationHeight);
  const integrations = renderIntegrationPills({
    integrations: data.integrations ?? [],
    x: 0,
    y: integrationsY,
    width,
    colors,
    pillHeight: INTEGRATION_PILL_HEIGHT,
    rowGap: INTEGRATION_ROW_GAP,
    maxRows: maxIntegrationRows,
  });

  const integrationsDividerY = integrationsY + integrations.height + INTEGRATIONS_DIVIDER_GAP;

  const avatar = renderAvatar({
    avatar: data.avatar,
    x: avatarX,
    y: avatarY,
    size: avatarSize,
    clipId: `avatar-clip-${uid}`,
    borderColor: colors.glow,
    borderWidth: 1,
    radius: 8,
  });

  const nameClipId = `name-clip-${uid}`;
  const nameClipY = nameY - nameFontSize;
  const nameClipHeight = nameLineHeight * nameLines.length + Math.round(nameFontSize * 0.35);
  const nameSvg = nameLines.map((line, index) =>
    `<text x="${width / 2}" y="${nameY + index * nameLineHeight}" font-family="system-ui, sans-serif" font-size="${nameFontSize}" font-weight="600" fill="${colors.foreground}" text-anchor="middle" letter-spacing="0" clip-path="url(#${nameClipId})">${escapeXml(line)}</text>`
  ).join("\n    ");

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
  <defs>
    <clipPath id="card-clip-${uid}"><rect width="${width}" height="${height}" rx="${r}"/></clipPath>
    <mask id="banner-mask-${uid}">
      <rect width="${width}" height="${bannerHeight}" fill="white"/>
      <rect x="${(width - 40) / 2}" y="${(bannerHeight - 12) / 2}" width="40" height="12" rx="6" fill="black"/>
    </mask>
    <clipPath id="${nameClipId}"><rect x="${NAME_MAX_WIDTH_PAD}" y="${nameClipY}" width="${nameMaxWidth}" height="${nameClipHeight}"/></clipPath>
    ${avatar?.defs ?? ""}
  </defs>
  <g clip-path="url(#card-clip-${uid})">
    <rect width="${width}" height="${height}" fill="${colors.background}"/>
    <rect width="${width}" height="${bannerHeight}" fill="${colors.accent}" mask="url(#banner-mask-${uid})"/>
    ${avatar?.content ?? ""}
    ${nameSvg}
    <line x1="0" y1="${nameDividerY}" x2="${width}" y2="${nameDividerY}" stroke="${colors.glow}" stroke-width="1" opacity="0.15"/>
    ${stats.content}
    ${stats.height > 0 ? `<line x1="0" y1="${statsDividerY}" x2="${width}" y2="${statsDividerY}" stroke="${colors.glow}" stroke-width="1" opacity="0.15"/>` : ""}
    ${integrations.content}
    ${integrations.height > 0 ? `<line x1="0" y1="${integrationsDividerY}" x2="${width}" y2="${integrationsDividerY}" stroke="${colors.glow}" stroke-width="1" opacity="0.15"/>` : ""}
    ${barcode.content}
  </g>
  <rect width="${width}" height="${height}" rx="${r}" fill="none" stroke="${colors.glow}" stroke-width="1" opacity="0.3"/>
</svg>`;
}
