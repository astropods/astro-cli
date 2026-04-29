import path from "path";
import {
  buildBlueprintBadgeSvg,
  renderSvgToPng,
  resolveAvatar,
  type CardColors,
} from "@astropods/blueprint-jellybean";

const API_URL = process.env.API_URL || "http://localhost:8080";
const STATIC_DIR = path.resolve("./build/client");

// ─── API types ────────────────────────────────────────────────────────────────

interface ApiAvatarColors {
  background: string;
  foreground: string;
  accent:     string;
  glow:       string;
}

interface ApiBlueprint {
  name:           string;
  account:        string;
  visibility?:    string;
  avatar_url?:    string;
  avatar_colors?: ApiAvatarColors;
  versions: Array<{ agent_card?: { description?: string } }>;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const DEFAULT_COLORS: CardColors = {
  background: "#041F1F",
  foreground: "#ffffff",
  accent:     "#14b8a6",
  glow:       "#99f6e4",
};

function mapColors(ac: ApiAvatarColors | undefined): CardColors {
  if (!ac?.background || !ac.foreground || !ac.accent || !ac.glow) return DEFAULT_COLORS;
  return { background: ac.background, foreground: ac.foreground, accent: ac.accent, glow: ac.glow };
}

const ACCOUNT_RE = /^[a-z0-9][a-z0-9-]*$/;
const NAME_RE    = /^[a-z0-9][a-z0-9._-]*$/;

function negCache(status: number): Response {
  return new Response(null, { status, headers: { "Cache-Control": "public, max-age=300" } });
}

// ─── Route loader ─────────────────────────────────────────────────────────────

export async function loader({ request }: { request: Request }) {
  const { pathname } = new URL(request.url);
  const match = pathname.match(/^\/badge\/agents\/([^/]+)\/([^/]+)\.png$/);
  if (!match) return negCache(404);

  const account = decodeURIComponent(match[1]);
  const name    = decodeURIComponent(match[2]);

  if (
    !ACCOUNT_RE.test(account) || account.length > 64 ||
    !NAME_RE.test(name)       || name.length > 64
  ) return negCache(404);

  // Fetch blueprint metadata from the Go API
  let bp: ApiBlueprint;
  try {
    const ctrl  = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), 5000);
    let res: Response;
    try {
      res = await fetch(
        `${API_URL}/api/v1/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}`,
        { signal: ctrl.signal },
      );
    } finally {
      clearTimeout(timer);
    }
    if (!res.ok) return negCache(res.status === 404 ? 404 : 502);
    bp = (await res.json()) as ApiBlueprint;
  } catch {
    return negCache(502);
  }

  if (bp.visibility !== "public" || bp.versions.length === 0) return negCache(404);

  const colors = mapColors(bp.avatar_colors);

  const agentAvatarPath   = bp.avatar_url
    ?? `/assets/avatars/agents/${encodeURIComponent(account)}/${encodeURIComponent(name)}.jpg`;
  const accountAvatarPath = `/assets/avatars/${encodeURIComponent(account)}.jpg`;

  const [agentAvatarUri, accountAvatarUri] = await Promise.all([
    resolveAvatar(agentAvatarPath,   API_URL, STATIC_DIR),
    resolveAvatar(accountAvatarPath, API_URL, STATIC_DIR),
  ]);

  let png: Buffer;
  try {
    const svg = buildBlueprintBadgeSvg(
      { name, account, description: bp.versions[0]?.agent_card?.description ?? "", agentAvatarUri, accountAvatarUri },
      colors,
    );
    png = renderSvgToPng(svg);
  } catch {
    return negCache(500);
  }

  return new Response(new Uint8Array(png), {
    headers: { "Content-Type": "image/png", "Cache-Control": "public, max-age=31536000, immutable" },
  });
}
