#!/usr/bin/env node
// Fails if any docs/03-architecture/*.md is missing its Status/Last-verified
// stamp. docs/README.md's rule requires both as a doc's first two lines
// (either order: some docs put the H1 title first, then the stamp; both
// are accepted, only presence is checked, not position).

const { readdirSync, readFileSync } = await import("node:fs");
const { join } = await import("node:path");

const DIR = new URL("../docs/03-architecture", import.meta.url).pathname;

const missing = [];
for (const entry of readdirSync(DIR)) {
  if (!entry.endsWith(".md")) continue;
  const text = readFileSync(join(DIR, entry), "utf8");
  const hasStatus = /^\*\*Status:\*\*/m.test(text);
  const hasLastVerified = /^\*\*Last verified:\*\*/m.test(text);
  if (!hasStatus || !hasLastVerified) missing.push(entry);
}

if (missing.length > 0) {
  console.error("Missing Status/Last verified stamp:");
  missing.forEach((f) => console.error(`  docs/03-architecture/${f}`));
  console.error(
    "\nAdd both as the doc's first two lines (see docs/README.md's rule) before merging.",
  );
  process.exit(1);
}

console.log(`All 03-architecture docs carry a Status/Last verified stamp.`);
