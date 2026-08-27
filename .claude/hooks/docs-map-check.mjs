#!/usr/bin/env node
// Stop hook: nudges a doc check when a mapped code path changed without its
// doc. Never blocks; internal errors go to stderr.

import { readFileSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

// Derived from this file's own path.
const REPO_ROOT = dirname(dirname(dirname(fileURLToPath(import.meta.url))));

function readStdin() {
  try {
    return readFileSync(0, "utf8");
  } catch {
    return "";
  }
}

// Minimal glob→regex: ** across segments, * within one.
export function globToRegExp(glob) {
  const escaped = glob
    .replace(/[.+^${}()|[\]\\]/g, "\\$&")
    .replace(/\*\*/g, " ")
    .replace(/\*/g, "[^/]*")
    .replace(/ /g, ".*");
  return new RegExp(`^${escaped}$`);
}

// Separate from parseAreaTable so main() can tell a missing heading from an empty table.
export function findAreaMapSection(readmeText) {
  const marker = "## Area → canonical doc map";
  const idx = readmeText.indexOf(marker);
  return idx === -1 ? null : readmeText.slice(idx + marker.length);
}

export function parseAreaTable(readmeText) {
  const section = findAreaMapSection(readmeText);
  if (section === null) return [];
  const tableText = section.split(/\n## /)[0];
  const rows = tableText
    .split("\n")
    .filter((line) => line.trim().startsWith("|"))
    .slice(2); // drop the header row and the `---` separator row

  return rows
    .map((row) => {
      const cells = row
        .split("|")
        .slice(1, -1)
        .map((c) => c.trim());
      const [area, codePaths, docs] = cells;
      if (area === undefined || codePaths === undefined || docs === undefined) return null;
      const pathGlobs = (codePaths.match(/`([^`]+)`/g) ?? []).map((s) => s.slice(1, -1));
      const docPaths = (docs.match(/\]\(([^)]+)\)/g) ?? []).map((s) => s.slice(2, -1).split("#")[0]);
      return { area, pathGlobs, docPaths };
    })
    .filter((row) => row !== null);
}

// Edit/Write/MultiEdit paths since the last user entry. MAX_SCAN guards
// against a transcript with no user entry at all.
const MAX_SCAN_LINES = 5000;

export function collectEditedPathsThisTurn(transcriptPath) {
  const lines = readFileSync(transcriptPath, "utf8").trim().split("\n");
  const paths = new Set();
  let truncated = false;

  for (let i = lines.length - 1, scanned = 0; i >= 0; i--, scanned++) {
    if (scanned >= MAX_SCAN_LINES) {
      truncated = true;
      break;
    }
    let entry;
    try {
      entry = JSON.parse(lines[i]);
    } catch {
      continue;
    }
    if (entry.type === "user") break;
    if (entry.type !== "assistant") continue;

    const content = entry.message?.content;
    if (!Array.isArray(content)) continue;
    for (const block of content) {
      if (block.type !== "tool_use") continue;
      if (!["Edit", "Write", "MultiEdit"].includes(block.name)) continue;
      const filePath = block.input?.file_path;
      if (filePath) paths.add(filePath);
    }
  }
  return { paths, truncated };
}

function relativeToRepo(cwd, absolutePath) {
  const rel = absolutePath.startsWith(cwd) ? absolutePath.slice(cwd.length + 1) : absolutePath;
  return rel.replace(/\\/g, "/");
}

// editedPaths must already be repo-root-relative (see relativeToRepo).
export function flagAreas(table, editedPaths) {
  const flagged = [];
  for (const { area, pathGlobs, docPaths } of table) {
    const patterns = pathGlobs.map(globToRegExp);
    const touchedCode = [...editedPaths].some((p) => patterns.some((re) => re.test(p)));
    if (!touchedCode) continue;

    const docTouched = docPaths.some((docPath) => editedPaths.has(`docs/${docPath}`));
    if (!docTouched) flagged.push({ area, docPaths });
  }
  return flagged;
}

function main() {
  const raw = readStdin();
  if (!raw) return;
  let input;
  try {
    input = JSON.parse(raw);
  } catch {
    return;
  }

  const readmePath = join(REPO_ROOT, "docs", "README.md");
  if (!input.transcript_path || !existsSync(input.transcript_path) || !existsSync(readmePath)) return;

  const readmeText = readFileSync(readmePath, "utf8");
  if (findAreaMapSection(readmeText) === null) {
    process.stderr.write(
      'docs-map-check: could not find the "## Area → canonical doc map" heading ' +
        "in docs/README.md — matching zero areas until this is fixed (the heading " +
        "text likely changed and this parser wasn't updated to match).\n",
    );
    return;
  }

  const table = parseAreaTable(readmeText);
  const { paths: editedAbs, truncated } = collectEditedPathsThisTurn(input.transcript_path);
  if (truncated) {
    process.stderr.write(
      `docs-map-check: scanned ${MAX_SCAN_LINES} transcript lines without finding this turn's ` +
        "start — coverage for this turn may be incomplete.\n",
    );
  }
  if (editedAbs.size === 0) return;

  const edited = new Set([...editedAbs].map((p) => relativeToRepo(REPO_ROOT, p)));
  const flagged = flagAreas(table, edited);
  if (flagged.length === 0) return;

  const lines = flagged.map(
    ({ area, docPaths }) => `- ${area}: ${docPaths.map((d) => `docs/${d}`).join(", ")}`,
  );
  const context =
    "This turn edited code in a documented area without touching its canonical doc(s). " +
    "Per docs/README.md's rule, check whether the change makes any of these stale, and fix " +
    "in place if so (or log to docs/07-feedback/doc-drift-log.md if the fix needs follow-up):\n" +
    lines.join("\n");

  process.stdout.write(
    JSON.stringify({
      hookSpecificOutput: { hookEventName: "Stop", additionalContext: context },
    }),
  );
}

// Skip main() when imported (docs-map-check.test.mjs imports the functions above).
if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    main();
  } catch (err) {
    process.stderr.write(`docs-map-check: unexpected error: ${err.stack}\n`);
  }
}
