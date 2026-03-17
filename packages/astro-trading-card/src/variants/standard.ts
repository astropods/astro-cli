import { CARD_DIMENSIONS, type CardColors, type CardData } from "../types";
import { renderAvatar, renderStatRows, renderIntegrationPills, escapeXml } from "../svg";
import { renderBarcode } from "../barcode";

export function renderStandard(data: CardData, colors: CardColors): string {
  const { width, height } = CARD_DIMENSIONS.standard;
  const bannerHeight = Math.round(height * 0.075);
  const r = 16;

  const nameText = (data.displayName ?? data.name).toUpperCase();
  const nameFontSize = Math.round(Math.min(32, Math.max(20, 400 / nameText.length)));

  const avatarSize = 120;
  const avatarX = (width - avatarSize) / 2;
  const avatarY = bannerHeight + 20;
  const nameGap = nameFontSize + 16;
  const nameY = avatarY + avatarSize + nameGap;
  const nameDividerY = nameY + Math.round(nameFontSize * 0.5) + 14;
  const statsY = nameDividerY;
  const stats = renderStatRows({
    stats: data.stats ?? [],
    x: 0,
    y: statsY,
    width,
    colors,
  });

  const statsDividerY = statsY + stats.height;
  const integrationsY = statsDividerY + 12;
  const integrations = renderIntegrationPills({
    integrations: data.integrations ?? [],
    x: 0,
    y: integrationsY,
    width,
    colors,
  });

  const integrationsDividerY = integrationsY + integrations.height + 12;

  const barcodeBottomPadding = 16;
  const barcodeMeasure = renderBarcode({ id: data.barcodeId ?? "", x: 0, y: 0, width, colors });
  const barcodeY = height - barcodeMeasure.height - barcodeBottomPadding;
  const barcode = renderBarcode({ id: data.barcodeId ?? "", x: 0, y: barcodeY, width, colors });

  const avatar = renderAvatar({
    avatar: data.avatar,
    x: avatarX,
    y: avatarY,
    size: avatarSize,
    clipId: "avatar-clip",
    borderColor: colors.glow ?? colors.foreground,
    borderWidth: 1,
    radius: 8,
  });

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
  <defs>
    <clipPath id="card-clip"><rect width="${width}" height="${height}" rx="${r}"/></clipPath>
    <mask id="banner-mask">
      <rect width="${width}" height="${bannerHeight}" fill="white"/>
      <rect x="${(width - 40) / 2}" y="${(bannerHeight - 12) / 2}" width="40" height="12" rx="6" fill="black"/>
    </mask>
    ${avatar?.defs ?? ""}
  </defs>
  <g clip-path="url(#card-clip)">
    <rect width="${width}" height="${height}" fill="${colors.background}"/>
    <rect width="${width}" height="${bannerHeight}" fill="${colors.accent}" mask="url(#banner-mask)"/>
    ${avatar?.content ?? ""}
    <text x="${width / 2}" y="${nameY}" font-family="system-ui, sans-serif" font-size="${nameFontSize}" font-weight="600" fill="${colors.foreground}" text-anchor="middle" letter-spacing="0.05em">${escapeXml(nameText)}</text>
    <line x1="0" y1="${nameDividerY}" x2="${width}" y2="${nameDividerY}" stroke="${colors.glow ?? colors.foreground}" stroke-width="1" opacity="0.15"/>
    ${stats.content}
    ${stats.height > 0 ? `<line x1="0" y1="${statsDividerY}" x2="${width}" y2="${statsDividerY}" stroke="${colors.glow ?? colors.foreground}" stroke-width="1" opacity="0.15"/>` : ""}
    ${integrations.content}
    ${integrations.height > 0 ? `<line x1="0" y1="${integrationsDividerY}" x2="${width}" y2="${integrationsDividerY}" stroke="${colors.glow ?? colors.foreground}" stroke-width="1" opacity="0.15"/>` : ""}
    ${barcode.content}
  </g>
  <rect width="${width}" height="${height}" rx="${r}" fill="none" stroke="${colors.glow ?? colors.foreground}" stroke-width="1" opacity="0.3"/>
</svg>`;
}
