#!/usr/bin/env node
// Checks every relative markdown link and every symlink under docs/ resolves.
// Fails on anything new; known pre-existing breaks are exempted below so
// this doesn't need a full backlog cleanup before it can start catching
// regressions. Remove an exception once it's fixed.

import { readFileSync, readdirSync, existsSync } from "node:fs";
import { join, dirname, normalize, relative } from "node:path";

const REPO_ROOT = new URL("..", import.meta.url).pathname;
const DOCS_ROOT = join(REPO_ROOT, "docs");

// [file relative to repo root, link target exactly as written in the file]
const KNOWN_BROKEN_LINKS = new Set(
  [
    ["docs/releases/2026-07-10.1.md", "./docs/arch.png"],
    ["docs/changelog/sohum/slack-observed-users-table-2026-06-04.md", "../../proposals/slack-mappings-table-split.md"],
    ["docs/changelog/sohum/slack-observed-users-cutover-2026-06-04.md", "../../proposals/slack-mappings-table-split.md"],
    ["docs/changelog/sohum/slack-mappings-cleanup-2026-06-04.md", "../../proposals/slack-mappings-table-split.md"],
    ["docs/changelog/feat/cli-create-gateway-model-2026-06-30.md", "/ai-gateway"],
    ["docs/changelog/feat/ai-gateway-astro-server-2026-06-03.md", "../plans/ai-gateway-astro-server.md"],
    ["docs/changelog/feat/ai-gateway-astro-server-2026-06-03.md", "../plans/ai-gateway-dev-keys.md"],
    ["docs/changelog/feat/agent-md-image-assets-2026-06-24.md", "./docs/arch.png"],
    ["docs/changelog/sas/bugbash-may-1-2026-05-01.md", "../03-architecture/registry-token-auth.md"],
    ["docs/changelog/fix/lineage-validation-in-store-2026-04-30.md", "apps/astro-server/e2e/lineage_validation_test.go"],
    ["docs/06-plan/ai-gateway-astro-server.md", "../../modules/astro-infra/docs/plans/ai-gateway.md"],
    ["docs/06-plan/ai-gateway-astro-server.md", "ai-gateway.md"],
    ["docs/01-spec/multi-region-cluster-support-spec.md", "../../../../.cursor/plans/m0_phase_1_pr_sequence_60a44a07.plan.md"],
    ["docs/01-spec/machine-credentials-spec.md", "access-audiences-spec.md"],
    ["docs/01-spec/private-by-default-fgac-rollout.md", "access-audiences-spec.md"],
  ].map(([f, t]) => `${f} ${t}`),
);

function walk(dir, exts) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isSymbolicLink()) {
      out.push({ path: full, symlink: true });
      continue;
    }
    if (entry.isDirectory()) {
      out.push(...walk(full, exts));
    } else if (exts.some((e) => entry.name.endsWith(e))) {
      out.push({ path: full, symlink: false });
    }
  }
  return out;
}

const LINK_RE = /\]\(([^)]+)\)/g;
const failures = [];

for (const { path: filePath, symlink } of walk(DOCS_ROOT, [".md", ".mdx"])) {
  const relFile = relative(REPO_ROOT, filePath).replace(/\\/g, "/");

  if (symlink) {
    if (!existsSync(filePath)) {
      failures.push(`${relFile}: dangling symlink`);
    }
    continue;
  }

  const text = readFileSync(filePath, "utf8");
  for (const match of text.matchAll(LINK_RE)) {
    const target = match[1];
    if (/^(https?:|mailto:|#)/.test(target)) continue;
    if (target.startsWith("/")) continue; // site-absolute URL, not a repo path

    const targetClean = target.split("#")[0];
    if (!targetClean) continue;
    const resolved = normalize(join(dirname(filePath), targetClean));
    if (existsSync(resolved)) continue;

    if (KNOWN_BROKEN_LINKS.has(`${relFile} ${target}`)) continue;
    failures.push(`${relFile}: broken link -> ${target}`);
  }
}

if (failures.length > 0) {
  console.error(`${failures.length} broken doc link(s)/symlink(s) found:\n`);
  for (const f of failures) console.error(`  ${f}`);
  process.exit(1);
}

console.log("All doc links and symlinks resolve (or are known exceptions).");
