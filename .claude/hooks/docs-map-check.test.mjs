// Run with: node .claude/hooks/docs-map-check.test.mjs
// No test runner/dependency beyond Node's built-in node:test — matches the
// hook script's own dependency-free design.
import { test } from "node:test";
import assert from "node:assert/strict";
import { writeFileSync, unlinkSync, mkdtempSync, mkdirSync, copyFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import {
  globToRegExp,
  findAreaMapSection,
  parseAreaTable,
  collectEditedPathsThisTurn,
  flagAreas,
} from "./docs-map-check.mjs";

const SCRIPT_PATH = fileURLToPath(new URL("./docs-map-check.mjs", import.meta.url));
const REPO_ROOT = dirname(dirname(dirname(SCRIPT_PATH)));

test("globToRegExp: ** matches across segments, * within one", () => {
  assert.match("apps/astro-server/internal/billing/foo.go", globToRegExp("apps/astro-server/internal/billing/**"));
  assert.match("apps/astro-server/handlers/billing.go", globToRegExp("apps/astro-server/handlers/billing.go"));
  assert.match("apps/astro-client/src/lib/billing-balances.ts", globToRegExp("apps/astro-client/src/lib/billing-*.ts"));
  assert.doesNotMatch("apps/astro-client/src/lib/other/billing-balances.ts", globToRegExp("apps/astro-client/src/lib/billing-*.ts"));
  assert.doesNotMatch("apps/astro-server/internal/auth/jwt.go", globToRegExp("apps/astro-server/internal/billing/**"));
});

test("findAreaMapSection: null when the heading is missing or reworded", () => {
  assert.equal(findAreaMapSection("# docs/\n\nno such heading here"), null);
  assert.equal(findAreaMapSection("## Area -> canonical doc map\n\n| a | b |"), null); // arrow swapped
  assert.notEqual(findAreaMapSection("## Area → canonical doc map\n\n| a | b |"), null);
});

test("parseAreaTable: parses a real-shaped table", () => {
  const readme = `# docs/

## Area → canonical doc map

| Area | Code paths | Canonical doc(s) | Notes |
|---|---|---|---|
| Billing | \`apps/astro-server/internal/billing/**\`, \`apps/astro-server/handlers/billing.go\` | [\`03-architecture/billing-overview.md\`](03-architecture/billing-overview.md) | some notes here |

## How this stays current

more text
`;
  const table = parseAreaTable(readme);
  assert.equal(table.length, 1);
  assert.equal(table[0].area, "Billing");
  assert.deepEqual(table[0].pathGlobs, ["apps/astro-server/internal/billing/**", "apps/astro-server/handlers/billing.go"]);
  assert.deepEqual(table[0].docPaths, ["03-architecture/billing-overview.md"]);
});

test("parseAreaTable: strips a #anchor from a doc link", () => {
  const readme = `## Area → canonical doc map

| Area | Code paths | Canonical doc(s) | Notes |
|---|---|---|---|
| Billing | \`apps/astro-server/internal/billing/**\` | [\`x.md\`](x.md#some-section) | notes |

## Next
`;
  const table = parseAreaTable(readme);
  assert.deepEqual(table[0].docPaths, ["x.md"]);
});

test("parseAreaTable: [] (not a throw) when the heading is missing", () => {
  assert.deepEqual(parseAreaTable("# docs/\n\nnothing here"), []);
});

test("parseAreaTable: skips a malformed row instead of throwing", () => {
  const readme = `## Area → canonical doc map

| Area | Code paths | Canonical doc(s) | Notes |
|---|---|---|---|
| Broken row with too few cells |
| Billing | \`apps/astro-server/internal/billing/**\` | [\`x.md\`](x.md) | notes |

## Next
`;
  const table = parseAreaTable(readme);
  assert.equal(table.length, 1);
  assert.equal(table[0].area, "Billing");
});

test("parseAreaTable: [] when the section has no data rows", () => {
  const readme = `## Area → canonical doc map

| Area | Code paths | Canonical doc(s) | Notes |
|---|---|---|---|

## Next section
`;
  assert.deepEqual(parseAreaTable(readme), []);
});

test("collectEditedPathsThisTurn: stops at the most recent user boundary", () => {
  const lines = [
    JSON.stringify({ type: "user" }), // an earlier turn — must not be scanned
    JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "tool_use", name: "Edit", input: { file_path: "/repo/should-not-appear.go" } }] },
    }),
    JSON.stringify({ type: "user" }), // this turn's boundary
    JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "tool_use", name: "Edit", input: { file_path: "/repo/docs/README.md" } }] },
    }),
    JSON.stringify({
      type: "assistant",
      message: { content: [{ type: "tool_use", name: "Read", input: { file_path: "/repo/ignored.go" } }] },
    }),
  ];
  const tmp = join(tmpdir(), `docs-map-check-test-${process.pid}.jsonl`);
  writeFileSync(tmp, lines.join("\n"));
  try {
    const { paths, truncated } = collectEditedPathsThisTurn(tmp);
    assert.equal(truncated, false);
    assert.deepEqual([...paths], ["/repo/docs/README.md"]);
  } finally {
    unlinkSync(tmp);
  }
});

test("collectEditedPathsThisTurn: truncated=true when no user boundary exists in range", () => {
  const many = Array.from({ length: 5010 }, () => JSON.stringify({ type: "assistant", message: { content: [] } }));
  const tmp = join(tmpdir(), `docs-map-check-test-trunc-${process.pid}.jsonl`);
  writeFileSync(tmp, many.join("\n"));
  try {
    const { truncated } = collectEditedPathsThisTurn(tmp);
    assert.equal(truncated, true);
  } finally {
    unlinkSync(tmp);
  }
});

const BILLING_ROW = {
  area: "Billing",
  pathGlobs: ["apps/astro-server/internal/billing/**"],
  docPaths: ["03-architecture/billing-overview.md", "03-architecture/billing-data-flow.md"],
};
const AUTH_ROW = {
  area: "Auth",
  pathGlobs: ["apps/astro-server/internal/auth/**"],
  docPaths: ["03-architecture/authentication-flow.md"],
};

test("flagAreas: two areas touched, only one has its doc updated", () => {
  const edited = new Set([
    "apps/astro-server/internal/billing/status.go",
    "apps/astro-server/internal/auth/jwt.go",
    "docs/03-architecture/billing-overview.md",
  ]);
  const flagged = flagAreas([BILLING_ROW, AUTH_ROW], edited);
  assert.deepEqual(
    flagged.map((f) => f.area),
    ["Auth"],
  );
});

test("flagAreas: touching any one of several canonical docs satisfies the area", () => {
  const edited = new Set([
    "apps/astro-server/internal/billing/status.go",
    "docs/03-architecture/billing-data-flow.md",
  ]);
  assert.deepEqual(flagAreas([BILLING_ROW], edited), []);
});

test("flagAreas: an unrelated file ending in the same doc suffix must not count as touched", () => {
  const edited = new Set([
    "apps/astro-server/internal/billing/status.go",
    "docs-public/fern/docs/pages/03-architecture/billing-overview.md",
  ]);
  const flagged = flagAreas([BILLING_ROW], edited);
  assert.equal(flagged.length, 1);
  assert.equal(flagged[0].area, "Billing");
});

test("main(): a corrupt docs/README.md never crashes the process", () => {
  // Isolated copy so this never touches the real repo's docs/README.md.
  const tmpRoot = mkdtempSync(join(tmpdir(), "docs-map-check-corrupt-"));
  mkdirSync(join(tmpRoot, ".claude", "hooks"), { recursive: true });
  mkdirSync(join(tmpRoot, "docs"), { recursive: true });
  copyFileSync(SCRIPT_PATH, join(tmpRoot, ".claude", "hooks", "docs-map-check.mjs"));
  writeFileSync(
    join(tmpRoot, "docs", "README.md"),
    "## Area → canonical doc map\n\n| Area | Code paths | Canonical doc(s) | Notes |\n|---|---|---|---|\n| Broken |\n",
  );
  const transcript = join(tmpRoot, "t.jsonl");
  writeFileSync(
    transcript,
    [
      JSON.stringify({ type: "user" }),
      JSON.stringify({
        type: "assistant",
        message: { content: [{ type: "tool_use", name: "Edit", input: { file_path: "/tmp/whatever.go" } }] },
      }),
    ].join("\n"),
  );
  try {
    const result = spawnSync(process.execPath, [join(tmpRoot, ".claude", "hooks", "docs-map-check.mjs")], {
      input: JSON.stringify({ cwd: tmpRoot, transcript_path: transcript }),
      encoding: "utf8",
    });
    assert.equal(result.status, 0);
  } finally {
    rmSync(tmpRoot, { recursive: true, force: true });
  }
});

test("main(): works when invoked with cwd set to a subdirectory", () => {
  const lines = [
    { type: "user" },
    {
      type: "assistant",
      message: {
        content: [
          {
            type: "tool_use",
            name: "Edit",
            input: { file_path: join(REPO_ROOT, "apps/astro-server/internal/billing/status.go") },
          },
        ],
      },
    },
  ];
  const tmp = join(tmpdir(), `docs-map-check-test-subdir-${process.pid}.jsonl`);
  writeFileSync(tmp, lines.map((l) => JSON.stringify(l)).join("\n"));
  try {
    const result = spawnSync(process.execPath, [SCRIPT_PATH], {
      cwd: join(REPO_ROOT, "apps/astro-server"),
      input: JSON.stringify({ cwd: join(REPO_ROOT, "apps/astro-server"), transcript_path: tmp }),
      encoding: "utf8",
    });
    assert.equal(result.status, 0);
    const output = JSON.parse(result.stdout);
    assert.match(output.hookSpecificOutput.additionalContext, /Billing/);
  } finally {
    unlinkSync(tmp);
  }
});
