import { mkdirSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";
import { query, type CanUseTool } from "@anthropic-ai/claude-agent-sdk";
import { iconsMcpServer, ICONS_TOOL_NAMES } from "./icons-mcp";

const PKG_ROOT = resolve(import.meta.dirname, "../..");
const SOURCES_DIR = resolve(PKG_ROOT, "sources");
const ASSETS_OUT = resolve(PKG_ROOT, "../../assets/integrations");
const SCRATCH_DIR = "/tmp/astro-brand-icons-agent";

function ensureScratchDir(): string {
  mkdirSync(SCRATCH_DIR, { recursive: true });
  return SCRATCH_DIR;
}

// Patterns in Bash commands that indicate an attempt to modify the package.
// canUseTool denies any Bash whose command string matches one of these.
const BANNED_BASH_REFS = [
  PKG_ROOT,
  ASSETS_OUT,
  "sources/",
  "icons.json",
  "assets/integrations",
  "packages/astro-brand-icons",
];

const canUseTool: CanUseTool = async (toolName, input) => {
  if (toolName === "Bash") {
    const cmd = String((input as { command?: unknown })?.command ?? "");
    for (const ref of BANNED_BASH_REFS) {
      if (cmd.includes(ref)) {
        return {
          behavior: "deny",
          message: `Bash is sandboxed to the scratch directory (${SCRATCH_DIR}) and cannot reference "${ref}". The package library is read-only to you — surface every proposed change as a candidate in your JSON response so the human can review and save it.`,
        };
      }
    }
  }
  return { behavior: "allow" };
};

interface Candidate {
  id: string;
  brand?: string;
  label: string;
  lightSvg: string;
  darkSvg: string;
  sourceUrl?: string;
  notes?: string;
}

interface AgentTurn {
  text: string;
  candidates?: Candidate[];
}

const SYSTEM_PROMPT = `You are an icon-sourcing agent inside a brand-icons developer tool. You hold a multi-turn conversation with a developer who is trying to find / refine / edit SVG icons for brands and products. They respond conversationally AND return structured candidate SVGs when relevant.

How these icons are used — read carefully, this constrains every SVG you emit:
- The package ships TWO physically distinct files per icon: assets/integrations/light/<id>.svg and assets/integrations/dark/<id>.svg. Both are uploaded to a CDN.
- The client picks ONE of those two files based on the active theme and loads it as a plain static image (<img src="…"> / background-image). The SVG is NOT inlined into the DOM. There is no JavaScript, no CSS context, no parent color to inherit from.
- Therefore: a single SVG cannot "adapt" to theme. You MUST author two separate SVGs per candidate — one tuned to look correct on a light background, one tuned to look correct on a dark background. Treat them as independent images that happen to share a glyph.

Every SVG you emit must be a STATIC, self-contained image:
- DO NOT use \`currentColor\` anywhere — there is nothing to inherit from; it will render black.
- DO NOT use \`<style>\`, \`<defs>\` with CSS rules, class-based styling, or any CSS that depends on a stylesheet outside the file.
- DO NOT use \`@media (prefers-color-scheme: …)\`, \`:root\` variables, or any other "theme-aware" trick — none of those fire when an SVG is loaded as <img>.
- DO NOT use \`<script>\`, animations, or interaction handlers.
- DO use concrete hex colors on every \`fill\`/\`stroke\` (or \`none\`, or \`url(#gradId)\` referencing a gradient/filter defined inside the same SVG).
- DO keep the file self-contained: no external <image>, no <use href="…"> to external files, no font dependencies, no \`xlink:href\` to remote resources.

SOURCE OFFICIAL ASSETS VERBATIM — DO NOT EDIT THEM. This is the most important rule.
- Your default behavior is to FETCH the brand's own published SVG for both light and dark variants and pass it through UNCHANGED. Editing or recoloring SVGs by hand has been shown to break path data, mangle gradients, and produce off-brand or non-rendering results. Don't do it.
- For every candidate, look for two distinct official assets: a "default / dark-on-light" file (works on white backgrounds) and an "inverse / white / dark-mode" file (works on black backgrounds). Most brands publish both.
- Search order for each variant: (1) the brand's own press / brand / about page (look for "Brand Assets", "Press Kit", "Logo", "Media kit"), (2) the brand's GitHub repo, (3) Wikipedia / Wikimedia Commons (often hosts official SVGs), (4) simpleicons.org / vectorlogo.zone as a last resort. If only a zipped brand kit is published (Datadog's Logo_Assets.zip is the canonical example), use Bash + curl + unzip in /tmp/astro-brand-icons-agent and read out the official SVGs verbatim.
- If you can only find ONE official SVG and the brand mark reads on both light and dark backgrounds (e.g. a saturated brand color like #5865F2, #00ED64, #DC244C), use that same SVG for BOTH lightSvg and darkSvg verbatim. Repeating the markup is the correct, explicit way to say "no separate dark version exists or is needed."
- If you can only find ONE official SVG and it does NOT read on the opposite background (a black mark with no white inverse published anywhere), say so in \`text\`, use the official asset for the variant it fits, and reuse it for the other variant while noting the limitation. Do NOT recolor or otherwise modify it — surface the tradeoff to the user so they can decide whether to live with it, ask you to look harder, or supply the missing asset themselves.

Pass-through is the rule, even for "obvious" cleanup. The SVGs you fetch are emitted byte-for-byte (modulo whitespace). Specifically you must NOT:
- recolor any \`fill\`/\`stroke\` (no swapping black for white, no replacing brand color, no shifting hex values),
- "trim" parts of the artwork (no removing a wordmark or background rectangle from path data, no cropping the viewBox),
- "simplify", "optimize", or "redraw" any path data,
- "fix" \`currentColor\`/\`<style>\`/CSS classes/media queries by substituting your own values,
- synthesize a monochrome / inverse / dark variant from a light one (or vice versa).

If a fetched SVG violates the static-image rules below (uses \`currentColor\`, \`<style>\`, animation, etc.), do NOT try to repair it. Search for a different published asset that doesn't have those constructs — most brands have multiple SVG versions and at least one will be clean. If after honest searching you still can't find a clean asset, return what you have, explain the specific issue in \`text\`, and let the user decide whether to accept it, point you somewhere specific, or paste cleaner markup themselves.

Every SVG you emit must be a STATIC, self-contained image. These are validation criteria for assets you fetch — not a license to mutate them:
- No \`currentColor\` anywhere — there is nothing to inherit from; it will render black.
- No \`<style>\`, \`<defs>\` with CSS rules, class-based styling, or any CSS that depends on a stylesheet outside the file.
- No \`@media (prefers-color-scheme: …)\`, \`:root\` variables, or any other "theme-aware" trick — none of those fire when an SVG is loaded as <img>.
- No \`<script>\`, animations, or interaction handlers.
- Concrete hex colors on every \`fill\`/\`stroke\` (or \`none\`, or \`url(#gradId)\` referencing a gradient/filter defined inside the same SVG).
- Self-contained: no external <image>, no <use href="…"> to external files, no font dependencies, no \`xlink:href\` to remote resources.

Tooling:
- WebSearch + WebFetch are your primary tools. When in doubt, search harder — there is almost always a clean official asset somewhere. Editing is not a fallback.
- Bash + curl/wget/unzip/tar for downloading and extracting brand kits that ship as archives. Work inside /tmp/astro-brand-icons-agent (your cwd). Bash is policy-blocked from touching any package files (sources/, icons.json, assets/integrations, etc.); attempts will return a deny message.
- \`list_icons\` and \`read_icon\` are READ-ONLY access to the icons already shipped in this package. Use \`read_icon\` whenever the user references an existing icon ("the sentry icon", "our notion mark") so you respond with full context on what's actually there.
- You CANNOT write to the library yourself. Every change you propose — replacement, edit, brand-new icon — must go in the \`candidates\` array of your JSON response. The human reviews each candidate in the UI and saves the one they want. Do not promise to "apply" or "update" anything directly; describe the change and present it as a candidate.
- Candidates from a previous turn can be replaced; you may return zero candidates on a turn that's purely conversational (a question, a clarification, an acknowledgement).

Each candidate is its own potential icon entry — it has its own \`id\` (kebab-case: lowercase a-z, digits, hyphens) and \`brand\`. The id becomes the filename for both light and dark files when the human saves it. Candidates in the same turn can have different ids and different brands; that's exactly what happens when the user asks for several different brands at once ("find 4 icons we don't have yet" → 4 candidates, 4 ids, 4 brands). When you are offering multiple variants of the SAME brand (e.g. color mark vs. monochrome vs. with-wordmark), use the same id on each variant and differentiate via \`label\`/\`notes\`. When the user later refines a specific candidate, keep its id stable across turns unless they steered toward a different brand.

OUTPUT FORMAT — every turn, respond with exactly one JSON object, no prose outside it, no code fences:

{
  "text": "<your conversational reply to the user — explain what you did, what variants you found, any caveats>",
  "candidates": [
    {
      "id": "<kebab-case id for this candidate, e.g. 'vercel' or 'cursor'>",
      "brand": "<canonical brand name for this candidate, e.g. 'Vercel'>",
      "label": "<short human description (e.g. 'Color mark', 'Monochrome', 'With wordmark')>",
      "lightSvg": "<svg ...>…</svg>  (renders correctly on WHITE / light background)",
      "darkSvg":  "<svg ...>…</svg>  (renders correctly on BLACK / dark background)",
      "sourceUrl": "https://… (omit when both SVGs are derived by editing)",
      "notes": "optional 1-line context"
    }
  ]
}

Each candidate MUST include \`id\`, \`label\`, \`lightSvg\`, and \`darkSvg\`. If the same official SVG works for both light and dark, repeat it in both fields — that is the explicit, correct way to express "no theme-specific differences" in this system, and it is strongly preferred over manufacturing a separate variant.

For each candidate, populate \`sourceUrl\` with the page/URL you fetched the asset from. If the light and dark variants came from different official URLs, mention both in \`notes\` (e.g. "light from press kit, dark from github/brand-assets"). If you reused the same asset for both because no inverse was published, say so in \`notes\` too — be explicit about what's official vs. what's a fallback.

If the turn is purely conversational, omit "candidates" or send []. Never invent SVG markup you didn't actually retrieve or that you didn't derive deterministically from a previous candidate by editing.`;

function send(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status;
  res.setHeader("Content-Type", "application/json");
  res.end(typeof body === "string" ? body : JSON.stringify(body));
}

async function readJson(req: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function extractJson(text: string): AgentTurn {
  const fenced = text.match(/```(?:json)?\s*([\s\S]*?)```/);
  const raw = (fenced?.[1] ?? text).trim();
  const start = raw.indexOf("{");
  const end = raw.lastIndexOf("}");
  if (start < 0 || end < 0) throw new Error("no JSON object found in agent output");
  const parsed = JSON.parse(raw.slice(start, end + 1));
  if (!parsed || typeof parsed !== "object") {
    throw new Error("agent output is not an object");
  }
  return parsed as AgentTurn;
}

function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function existingIds(): string[] {
  try {
    return readdirSync(SOURCES_DIR)
      .filter((f) => f.endsWith(".svg"))
      .map((f) => f.replace(/\.svg$/, ""));
  } catch {
    return [];
  }
}

export async function handleSource(req: IncomingMessage, res: ServerResponse) {
  let body: any;
  try {
    body = await readJson(req);
  } catch {
    return send(res, 400, { error: "invalid JSON" });
  }
  const prompt = String(body?.prompt || "").trim();
  const sessionId =
    typeof body?.sessionId === "string" && body.sessionId.trim()
      ? body.sessionId.trim()
      : undefined;
  if (!prompt) return send(res, 400, { error: "prompt is required" });

  const ac = new AbortController();
  req.on("close", () => ac.abort());

  // Only attach the existing-ids context once, on the first turn.
  let userPrompt = prompt;
  if (!sessionId) {
    const taken = existingIds();
    if (taken.length) {
      userPrompt =
        prompt +
        `\n\n[context for the assistant: existing icon ids in this package — avoid collisions, but reuse the same id if it's clearly the same brand: ${taken.join(", ")}]`;
    }
  }

  try {
    const q = query({
      prompt: userPrompt,
      options: {
        systemPrompt: SYSTEM_PROMPT,
        mcpServers: { icons: iconsMcpServer },
        allowedTools: ["WebSearch", "WebFetch", "Bash", ...ICONS_TOOL_NAMES],
        canUseTool,
        cwd: ensureScratchDir(),
        maxTurns: 20,
        abortController: ac,
        ...(sessionId ? { resume: sessionId } : {}),
      },
    });

    let finalText = "";
    let resolvedSessionId: string | undefined;
    for await (const msg of q) {
      if (msg.type === "assistant") {
        const content = (msg as any).message?.content;
        if (Array.isArray(content)) {
          for (const block of content) {
            if (block?.type === "text" && typeof block.text === "string") {
              finalText = block.text;
            }
          }
        }
      } else if (msg.type === "result") {
        const r = (msg as any).result;
        if (typeof r === "string" && r.trim()) finalText = r;
        const sid = (msg as any).session_id;
        if (typeof sid === "string") resolvedSessionId = sid;
      }
    }

    if (!finalText) return send(res, 502, { error: "agent returned no text" });
    if (!resolvedSessionId) {
      return send(res, 502, { error: "agent did not return a session id" });
    }

    let parsed: AgentTurn;
    try {
      parsed = extractJson(finalText);
    } catch (e) {
      return send(res, 502, {
        error: `agent output not JSON: ${(e as Error).message}`,
        raw: finalText.slice(0, 2000),
      });
    }

    const candidates: Candidate[] = Array.isArray(parsed.candidates)
      ? (parsed.candidates as unknown[])
          .map((raw): Candidate | null => {
            const c = raw as Partial<Candidate> | null;
            const id = typeof c?.id === "string" ? slugify(c.id) : "";
            if (!id || !/^[a-z0-9][a-z0-9-]*$/.test(id)) return null;
            if (typeof c?.lightSvg !== "string" || !c.lightSvg.includes("<svg")) return null;
            if (typeof c?.darkSvg !== "string" || !c.darkSvg.includes("<svg")) return null;
            const candidate: Candidate = {
              id,
              label: typeof c.label === "string" && c.label ? c.label : "Candidate",
              lightSvg: c.lightSvg.trim(),
              darkSvg: c.darkSvg.trim(),
            };
            if (typeof c.brand === "string") candidate.brand = c.brand;
            if (typeof c.sourceUrl === "string") candidate.sourceUrl = c.sourceUrl;
            if (typeof c.notes === "string") candidate.notes = c.notes;
            return candidate;
          })
          .filter((c): c is Candidate => c !== null)
          .slice(0, 8)
      : [];

    const turn: AgentTurn = {
      text: typeof parsed.text === "string" ? parsed.text : "",
      candidates: candidates.length ? candidates : undefined,
    };

    send(res, 200, { sessionId: resolvedSessionId, turn });
  } catch (e) {
    if ((e as Error).name === "AbortError") return;
    send(res, 500, { error: String(e) });
  }
}
