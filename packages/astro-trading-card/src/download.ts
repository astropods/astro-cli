/**
 * Browser-only utilities for downloading a trading card as SVG or PNG.
 */

export interface DownloadOptions {
  /** Agent name slug (e.g. "research-assistant"). */
  name: string;
  /** Agent or barcode ID (e.g. "AGT-7f3a9b2e-01"). */
  id: string;
}

function buildFilename(opts: DownloadOptions, ext: string): string {
  return `${opts.name}-${opts.id}.${ext}`;
}

/** Download a blob as a file. */
function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/** Convert an image URL to a data URI by fetching and reading as base64. */
async function urlToDataUri(url: string): Promise<string> {
  const res = await fetch(url, { mode: "cors" });
  const blob = await res.blob();
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onloadend = () => resolve(reader.result as string);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
  });
}

function resolveEmbeddableHref(href: string): string | null {
  if (!href || /^(data:|#)/i.test(href)) return null;
  try {
    return new URL(href, window.location.href).href;
  } catch {
    return null;
  }
}

/**
 * Find image hrefs in the SVG and replace them with
 * embedded data URIs so the SVG is fully self-contained.
 */
async function embedImages(svg: string): Promise<string> {
  const hrefRe = /<image\b[^>]*\b(?:href|xlink:href)="([^"]+)"[^>]*>/g;
  const matches = [...svg.matchAll(hrefRe)];
  if (matches.length === 0) return svg;

  const urls = [...new Set(matches.map((m) => m[1]))];
  const dataUris = new Map<string, string>();

  await Promise.all(
    urls.map(async (href) => {
      const url = resolveEmbeddableHref(href);
      if (!url) return;

      try {
        const dataUri = await urlToDataUri(url);
        dataUris.set(href, dataUri);
      } catch {
        // If fetch fails, leave the original href in place.
      }
    }),
  );

  let result = svg;
  for (const [url, dataUri] of dataUris) {
    result = result.replaceAll(`href="${url}"`, `href="${dataUri}"`);
  }
  return result;
}

/**
 * Download the card SVG as an .svg file.
 * Embeds any external images as data URIs so the file is self-contained.
 */
export async function downloadSvg(svg: string, opts: DownloadOptions): Promise<void> {
  const embedded = await embedImages(svg);
  const blob = new Blob([embedded], { type: "image/svg+xml" });
  downloadBlob(blob, buildFilename(opts, "svg"));
}

/**
 * Download the card SVG as a .png file.
 * Rasterizes via canvas at the given scale factor (default 2x for retina).
 */
export async function downloadPng(
  svg: string,
  opts: DownloadOptions,
  scale = 2,
): Promise<void> {
  // Embed images first so the canvas can render them
  const embedded = await embedImages(svg);

  const widthMatch = embedded.match(/width="(\d+)"/);
  const heightMatch = embedded.match(/height="(\d+)"/);
  const width = widthMatch ? parseInt(widthMatch[1], 10) : 400;
  const height = heightMatch ? parseInt(heightMatch[1], 10) : 560;

  const canvas = document.createElement("canvas");
  canvas.width = width * scale;
  canvas.height = height * scale;
  const ctx = canvas.getContext("2d")!;
  ctx.scale(scale, scale);

  const img = new Image();
  const blob = new Blob([embedded], { type: "image/svg+xml" });
  const url = URL.createObjectURL(blob);

  await new Promise<void>((resolve, reject) => {
    img.onload = () => {
      ctx.drawImage(img, 0, 0, width, height);
      URL.revokeObjectURL(url);
      resolve();
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("Failed to load SVG for rasterization"));
    };
    img.src = url;
  });

  const pngBlob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((b) => {
      if (b) resolve(b);
      else reject(new Error("Failed to create PNG blob"));
    }, "image/png");
  });

  downloadBlob(pngBlob, buildFilename(opts, "png"));
}
