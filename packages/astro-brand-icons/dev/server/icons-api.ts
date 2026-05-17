import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import type { IncomingMessage, ServerResponse } from "node:http";

const PKG_ROOT = resolve(import.meta.dirname, "../..");
const MANIFEST_PATH = join(PKG_ROOT, "icons.json");
const SOURCES_DIR = join(PKG_ROOT, "sources");
const ASSETS_OUT = resolve(PKG_ROOT, "../../assets/integrations");

function send(res: ServerResponse, status: number, body: unknown, contentType = "application/json") {
  res.statusCode = status;
  res.setHeader("Content-Type", contentType);
  res.end(typeof body === "string" || Buffer.isBuffer(body) ? body : JSON.stringify(body));
}

async function readJson(req: IncomingMessage): Promise<unknown> {
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function loadManifest() {
  return JSON.parse(readFileSync(MANIFEST_PATH, "utf8"));
}

function saveManifest(m: unknown) {
  writeFileSync(MANIFEST_PATH, JSON.stringify(m, null, 2) + "\n");
}

export function handleGetIcons(_req: IncomingMessage, res: ServerResponse) {
  send(res, 200, loadManifest());
}

export function handleGetSvg(
  req: IncomingMessage,
  res: ServerResponse,
  variant: "light" | "dark",
  id: string,
) {
  const file = join(ASSETS_OUT, variant, `${id}.svg`);
  if (!existsSync(file)) return send(res, 404, { error: "not found" });
  res.setHeader("Cache-Control", "no-store");
  send(res, 200, readFileSync(file), "image/svg+xml");
}

export async function handleSaveIcon(req: IncomingMessage, res: ServerResponse) {
  let body: any;
  try {
    body = await readJson(req);
  } catch {
    return send(res, 400, { error: "invalid JSON" });
  }
  const id = String(body?.id || "").trim();
  const lightSvg = String(body?.lightSvg || "");
  const darkSvg = String(body?.darkSvg || "");

  if (!/^[a-z0-9][a-z0-9-]*$/.test(id)) {
    return send(res, 400, { error: "id must be kebab-case (lowercase, digits, hyphens)" });
  }
  if (!lightSvg.trim().startsWith("<svg")) {
    return send(res, 400, { error: "lightSvg must start with <svg" });
  }
  if (!darkSvg.trim().startsWith("<svg")) {
    return send(res, 400, { error: "darkSvg must start with <svg" });
  }

  writeFileSync(join(SOURCES_DIR, `${id}.svg`), lightSvg.trim() + "\n");
  writeFileSync(join(SOURCES_DIR, `${id}.dark.svg`), darkSvg.trim() + "\n");

  // Upsert manifest entry.
  const manifest = loadManifest();
  const existingIdx = manifest.icons.findIndex((e: any) => e.id === id);
  const entry = { id };
  if (existingIdx >= 0) {
    manifest.icons[existingIdx] = entry;
  } else {
    manifest.icons.push(entry);
    manifest.icons.sort((a: any, b: any) => a.id.localeCompare(b.id));
  }
  saveManifest(manifest);

  const result = spawnSync(
    "bun",
    ["scripts/process.ts", "--id", id],
    { cwd: PKG_ROOT, encoding: "utf8" },
  );
  if (result.status !== 0) {
    return send(res, 500, {
      error: "processor failed",
      stdout: result.stdout,
      stderr: result.stderr,
    });
  }

  send(res, 200, { ok: true });
}

export async function handleProcess(req: IncomingMessage, res: ServerResponse) {
  let body: any = {};
  try {
    body = await readJson(req);
  } catch {
    // empty body is fine — means "process everything"
  }
  const id =
    typeof body?.id === "string" && /^[a-z0-9][a-z0-9-]*$/.test(body.id.trim())
      ? body.id.trim()
      : undefined;

  const args = ["scripts/process.ts"];
  if (id) args.push("--id", id);

  const started = Date.now();
  const result = spawnSync("bun", args, { cwd: PKG_ROOT, encoding: "utf8" });
  const durationMs = Date.now() - started;

  if (result.status !== 0) {
    return send(res, 500, {
      ok: false,
      error: "processor failed",
      durationMs,
      stdout: result.stdout,
      stderr: result.stderr,
    });
  }

  send(res, 200, {
    ok: true,
    durationMs,
    stdout: result.stdout?.trim(),
    stderr: result.stderr?.trim() || undefined,
  });
}
