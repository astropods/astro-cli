import type { IconsResponse, SourceResponse } from "./types";

export async function fetchIcons(): Promise<IconsResponse> {
  const res = await fetch("/api/icons");
  if (!res.ok) throw new Error(`fetchIcons: ${res.status}`);
  return res.json();
}

export function iconUrl(id: string, variant: "light" | "dark"): string {
  return `/svg/${variant}/${encodeURIComponent(id)}.svg?ts=${Date.now()}`;
}

export async function chatTurn(args: {
  prompt: string;
  sessionId?: string;
  signal?: AbortSignal;
}): Promise<SourceResponse> {
  const res = await fetch("/api/source", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ prompt: args.prompt, sessionId: args.sessionId }),
    signal: args.signal,
  });
  if (!res.ok) {
    const msg = await res.text().catch(() => "");
    throw new Error(`chatTurn failed: ${res.status} ${msg}`);
  }
  return res.json();
}

export interface ProcessResult {
  ok: true;
  durationMs: number;
  stdout?: string;
  stderr?: string;
}

export async function processAssets(id?: string): Promise<ProcessResult> {
  const res = await fetch("/api/process", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(id ? { id } : {}),
  });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(
      `processAssets failed: ${res.status} ${
        (json as any)?.stderr || (json as any)?.error || ""
      }`,
    );
  }
  return json as ProcessResult;
}

export async function saveIcon(payload: {
  id: string;
  lightSvg: string;
  darkSvg: string;
}): Promise<{ ok: true }> {
  const res = await fetch("/api/icons/save", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const msg = await res.text().catch(() => "");
    throw new Error(`saveIcon failed: ${res.status} ${msg}`);
  }
  return res.json();
}
