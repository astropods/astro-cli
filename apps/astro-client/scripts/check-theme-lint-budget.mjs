#!/usr/bin/env node
// Fails if local-theme/no-raw-theme-colors violations exceed BASELINE.
// The rule itself stays at 'warn' (60 pre-existing violations aren't being
// fixed in one pass) -- this only blocks the count from growing. Lower
// BASELINE in the same PR that fixes violations, to lock the improvement in.

const BASELINE = 51;

const { execFileSync } = await import("node:child_process");

// eslint exits non-zero when there's any error-level finding (a different
// rule, unrelated to this check) -- still capture stdout in that case rather
// than treating it as this script's own failure.
let output;
try {
  output = execFileSync("bun", ["x", "eslint", "src/", "-f", "json"], {
    encoding: "utf8",
    maxBuffer: 1024 * 1024 * 20,
    cwd: new URL("..", import.meta.url),
  });
} catch (err) {
  output = err.stdout;
}

const results = JSON.parse(output);
const count = results.reduce(
  (n, f) => n + f.messages.filter((m) => m.ruleId === "local-theme/no-raw-theme-colors").length,
  0,
);

if (count > BASELINE) {
  console.error(
    `local-theme/no-raw-theme-colors: ${count} violations, baseline is ${BASELINE}. ` +
      "Fix the new one(s) instead of raising the baseline.",
  );
  process.exit(1);
}

if (count < BASELINE) {
  console.log(
    `local-theme/no-raw-theme-colors: ${count} violations, baseline is still ${BASELINE}. ` +
      `The count improved -- lower BASELINE to ${count} in this PR to lock the win in, ` +
      "or it can silently regress back up to the old baseline later.",
  );
} else {
  console.log(`local-theme/no-raw-theme-colors: ${count} violations (baseline ${BASELINE}).`);
}
