#!/usr/bin/env node
// Reports which docs/README.md areas have code commits newer than their
// canonical 03-architecture doc's Last-verified date. Informational only:
// a commit touching a mapped path doesn't prove the doc is wrong, only that
// it's worth a look. Never blocks a PR -- this is meant to run on a
// schedule, not on pull_request, so a stale finding is a periodic signal to
// go re-verify, not a merge gate. Exits 1 when something is stale purely so
// the scheduled run shows as a visible (not silent) signal in CI history.

const { execFileSync } = await import("node:child_process");
const { readFileSync } = await import("node:fs");

const REPO_ROOT = new URL("..", import.meta.url).pathname;
const { parseAreaTable } = await import(
  new URL("../.claude/hooks/docs-map-check.mjs", import.meta.url)
);

const readmeText = readFileSync(`${REPO_ROOT}docs/README.md`, "utf8");
const table = parseAreaTable(readmeText);

function lastVerifiedDate(docPath) {
  const text = readFileSync(`${REPO_ROOT}docs/${docPath}`, "utf8");
  const match = text.match(/^\*\*Last verified:\*\*\s*(\d{4}-\d{2}-\d{2})/m);
  return match ? match[1] : null;
}

function mostRecentCommitDate(globs) {
  try {
    const out = execFileSync(
      "git",
      // %ad (author date), not %cd (committer date): a rebase bumps committer
      // date to the rebase time regardless of when the change was actually
      // made, which would falsely mark every doc stale after every rebase.
      ["log", "-1", "--format=%ad", "--date=short", "--", ...globs],
      { cwd: REPO_ROOT, encoding: "utf8" },
    ).trim();
    return out || null;
  } catch {
    return null;
  }
}

const stale = [];
const checked = [];

for (const { area, pathGlobs, docPaths } of table) {
  const architectureDocs = docPaths.filter((d) => d.startsWith("03-architecture/"));
  if (architectureDocs.length === 0) continue;

  const commitDate = mostRecentCommitDate(pathGlobs);
  if (!commitDate) continue;

  for (const docPath of architectureDocs) {
    const verified = lastVerifiedDate(docPath);
    if (!verified) continue; // check-doc-stamps.mjs catches a missing stamp separately
    checked.push({ area, docPath });
    if (commitDate > verified) {
      stale.push({ area, docPath, verified, commitDate });
    }
  }
}

console.log(`Checked ${checked.length} area/doc pairs across ${table.length} areas.`);

if (stale.length > 0) {
  console.log(`\n${stale.length} doc(s) have code commits newer than their Last-verified date:\n`);
  for (const { area, docPath, verified, commitDate } of stale) {
    console.log(`  ${area}`);
    console.log(`    docs/${docPath} — last verified ${verified}, code changed ${commitDate}`);
  }
  console.log(
    "\nThis doesn't mean any of these docs are wrong -- it means their mapped code paths " +
      "changed since someone last checked. Worth a look, not a required fix.",
  );
  process.exit(1);
}

console.log("Nothing stale.");
