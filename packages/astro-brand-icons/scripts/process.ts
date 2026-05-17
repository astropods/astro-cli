#!/usr/bin/env bun
/**
 * Process brand icon sources into normalized light/dark SVGs.
 *
 * Every icon has two source files in this package:
 *   sources/<id>.svg       — renders correctly on a LIGHT background
 *   sources/<id>.dark.svg  — renders correctly on a DARK background
 *
 * The processor normalizes both (strip title/desc/comments) and writes them
 * to <out>/light/<id>.svg and <out>/dark/<id>.svg. There is no recoloring
 * or theme-derivation logic — each variant is authored explicitly.
 *
 * Usage:
 *   bun scripts/process.ts                  # defaults to <repo>/assets/integrations
 *   bun scripts/process.ts --id <icon-id>   # only process one icon
 *   bun scripts/process.ts --out <path>     # override the output directory
 *
 * Paths are resolved relative to this file's location (via import.meta.url),
 * so the script works from any cwd and assumes its position in the monorepo
 * (packages/astro-brand-icons → ../../assets/integrations).
 */

import { readFileSync, writeFileSync, mkdirSync, existsSync, readdirSync } from "node:fs";
import { join, resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const PKG_ROOT = resolve(__dirname, "..");
const DEFAULT_OUT = resolve(PKG_ROOT, "../../assets/integrations");

interface IconEntry {
  id: string;
  brand?: string;
}

interface Manifest {
  icons: IconEntry[];
}

function parseArgs(argv: string[]): { out: string; only?: string } {
  let out = DEFAULT_OUT;
  let only: string | undefined;
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--out" && argv[i + 1]) {
      out = resolve(argv[++i]!);
    } else if (a.startsWith("--out=")) {
      out = resolve(a.slice("--out=".length));
    } else if (a === "--id" && argv[i + 1]) {
      only = argv[++i];
    } else if (a.startsWith("--id=")) {
      only = a.slice("--id=".length);
    }
  }
  return { out, only };
}

function normalize(svg: string): string {
  return svg
    .replace(/<title\b[^>]*>[\s\S]*?<\/title>/gi, "")
    .replace(/<desc\b[^>]*>[\s\S]*?<\/desc>/gi, "")
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/\s+\n/g, "\n")
    .replace(/<svg\b[^>]*>/, stripRootSizing)
    .trim();
}

// On the root <svg> only: drop intrinsic sizing (`width`/`height`) and any
// inline `style` attribute. Both are noise for a CDN-hosted icon — the host
// app controls the rendered size via CSS on the <img>, and inline styles
// on the root tag are almost always copy-pasted layout junk that does
// nothing when the file is loaded as a standalone image. Everything else
// (viewBox, xmlns, fill, preserveAspectRatio, fill-rule, role, etc.) is
// left untouched because it's part of the SVG's identity.
function stripRootSizing(rootTag: string): string {
  return rootTag
    .replace(/\s+width="[^"]*"/g, "")
    .replace(/\s+height="[^"]*"/g, "")
    .replace(/\s+style="[^"]*"/g, "")
    .replace(/\s{2,}/g, " ");
}

function loadManifest(): Manifest {
  return JSON.parse(readFileSync(join(PKG_ROOT, "icons.json"), "utf8")) as Manifest;
}

function writeFile(path: string, content: string): void {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, content + "\n");
}

function processIcon(entry: IconEntry, out: string): void {
  const lightSrc = join(PKG_ROOT, "sources", `${entry.id}.svg`);
  const darkSrc = join(PKG_ROOT, "sources", `${entry.id}.dark.svg`);
  if (!existsSync(lightSrc)) {
    throw new Error(`Missing light source for "${entry.id}" at ${lightSrc}`);
  }
  if (!existsSync(darkSrc)) {
    throw new Error(`Missing dark source for "${entry.id}" at ${darkSrc}`);
  }
  writeFile(join(out, "light", `${entry.id}.svg`), normalize(readFileSync(lightSrc, "utf8")));
  writeFile(join(out, "dark", `${entry.id}.svg`), normalize(readFileSync(darkSrc, "utf8")));
}

function checkSourcesParity(manifest: Manifest): void {
  const declared = new Set(manifest.icons.map((i) => i.id));
  const files = readdirSync(join(PKG_ROOT, "sources"));
  const lights = new Set(
    files.filter((f) => f.endsWith(".svg") && !f.endsWith(".dark.svg"))
      .map((f) => f.replace(/\.svg$/, "")),
  );
  const darks = new Set(
    files.filter((f) => f.endsWith(".dark.svg"))
      .map((f) => f.replace(/\.dark\.svg$/, "")),
  );

  const undeclared = [...lights].filter((id) => !declared.has(id));
  const missingLight = [...declared].filter((id) => !lights.has(id));
  const missingDark = [...declared].filter((id) => !darks.has(id));
  const orphanDark = [...darks].filter((id) => !lights.has(id));

  if (undeclared.length) {
    console.warn(`warning: sources without manifest entry: ${undeclared.join(", ")}`);
  }
  if (orphanDark.length) {
    console.warn(`warning: <id>.dark.svg with no matching <id>.svg: ${orphanDark.join(", ")}`);
  }
  if (missingLight.length) {
    throw new Error(`manifest entries missing <id>.svg: ${missingLight.join(", ")}`);
  }
  if (missingDark.length) {
    throw new Error(`manifest entries missing <id>.dark.svg: ${missingDark.join(", ")}`);
  }
}

function main(): void {
  const { out, only } = parseArgs(Bun.argv.slice(2));
  const manifest = loadManifest();
  checkSourcesParity(manifest);

  const entries = only
    ? manifest.icons.filter((e) => e.id === only)
    : manifest.icons;
  if (only && entries.length === 0) {
    throw new Error(`No manifest entry with id="${only}"`);
  }

  for (const entry of entries) processIcon(entry, out);

  console.log(`processed ${entries.length} icon(s) -> ${out}`);
}

main();
