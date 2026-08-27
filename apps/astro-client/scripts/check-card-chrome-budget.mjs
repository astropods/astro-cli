#!/usr/bin/env node
// Fails if hand-rolled `<Card>` chrome (a line combining bg-card + border +
// a rounded-* class) grows past BASELINE. `apps/astro-client/CLAUDE.md` says
// to import `Card` from `@/components/ui/card` instead of rolling
// `border border-border bg-card rounded-...` by hand -- a real AST-based
// lint rule for this would false-positive on legitimate bg-card usage that
// isn't card chrome (see the doc's own note on why this stays a ratchet,
// not a lint rule). Imperfect precision is fine for a ratchet: it only
// needs to count consistently, not flag every instance inline.
//
// Counts a line as hand-rolled chrome when it contains all three of
// `bg-card`, `border`, and a `rounded` token -- the same heuristic used to
// establish BASELINE, so re-running this script reproduces that count.

const BASELINE = 30;

const { readdirSync, readFileSync, statSync } = await import("node:fs");
const { join } = await import("node:path");

const SRC = new URL("../src", import.meta.url).pathname;

// components/ui/** is the primitive layer itself (Card's own implementation
// legitimately defines this chrome); stories/** is Storybook demo code, not
// production hand-rolling. Both excluded, matching the existing ratchet's
// allowlist categories (test files, stories, UI primitives).
const EXCLUDED_DIRS = ["components/ui", "stories"];

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    const stat = statSync(full);
    if (stat.isDirectory()) out.push(...walk(full));
    else if (entry.endsWith(".tsx") && !entry.endsWith(".test.tsx")) out.push(full);
  }
  return out;
}

let count = 0;
const hits = [];
for (const file of walk(SRC)) {
  const rel = file.split("/src/")[1];
  if (EXCLUDED_DIRS.some((dir) => rel.startsWith(`${dir}/`))) continue;
  const lines = readFileSync(file, "utf8").split("\n");
  lines.forEach((line, i) => {
    if (line.includes("bg-card") && line.includes("border") && /\brounded/.test(line)) {
      count++;
      hits.push(`${rel}:${i + 1}`);
    }
  });
}

if (count > BASELINE) {
  console.error(
    `Hand-rolled <Card> chrome: ${count} lines, baseline is ${BASELINE}. ` +
      "Use <Card> from @/components/ui/card for the new one(s) instead of raising the baseline.",
  );
  console.error(hits.join("\n"));
  process.exit(1);
}

if (count < BASELINE) {
  console.log(
    `Hand-rolled <Card> chrome: ${count} lines, baseline is still ${BASELINE}. ` +
      `The count improved -- lower BASELINE to ${count} in this PR to lock the win in, ` +
      "or it can silently regress back up to the old baseline later.",
  );
} else {
  console.log(`Hand-rolled <Card> chrome: ${count} lines (baseline ${BASELINE}).`);
}
