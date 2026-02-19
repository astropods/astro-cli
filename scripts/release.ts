#!/usr/bin/env bun

import { parseArgs } from "util";
import { resolve, dirname } from "path";

const ROOT_DIR = resolve(dirname(import.meta.dir));

// --- CLI flags ---

const { values: flags } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    bump: { type: "string" },
    "dry-run": { type: "boolean", default: false },
    yes: { type: "boolean", short: "y", default: false },
    "allow-branch": { type: "boolean", default: false },
  },
  strict: true,
});

const BUMP_OVERRIDE = flags.bump as "patch" | "minor" | "major" | undefined;
const DRY_RUN = flags["dry-run"]!;
const SKIP_CONFIRM = flags.yes!;
const ALLOW_BRANCH = flags["allow-branch"]!;

if (BUMP_OVERRIDE && !["patch", "minor", "major"].includes(BUMP_OVERRIDE)) {
  console.error(`Invalid bump type: ${BUMP_OVERRIDE}. Must be patch, minor, or major.`);
  process.exit(1);
}

const REGISTRY = "https://npm.pkg.github.com";

// --- Helpers ---

const c = {
  red: (s: string) => `\x1b[0;31m${s}\x1b[0m`,
  green: (s: string) => `\x1b[0;32m${s}\x1b[0m`,
  yellow: (s: string) => `\x1b[1;33m${s}\x1b[0m`,
  blue: (s: string) => `\x1b[0;34m${s}\x1b[0m`,
};

async function run(cmd: string[], opts: { cwd?: string; env?: Record<string, string> } = {}): Promise<string> {
  const proc = Bun.spawn(cmd, {
    cwd: opts.cwd ?? ROOT_DIR,
    stdout: "pipe",
    stderr: "pipe",
    env: { ...process.env, ...opts.env },
  });
  const stdout = await new Response(proc.stdout).text();
  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    const stderr = await new Response(proc.stderr).text();
    throw new Error(`Command failed (${exitCode}): ${cmd.join(" ")}\n${stderr}`);
  }
  return stdout.trim();
}

async function runPassthrough(cmd: string[], opts: { cwd?: string } = {}): Promise<void> {
  const proc = Bun.spawn(cmd, {
    cwd: opts.cwd ?? ROOT_DIR,
    stdout: "inherit",
    stderr: "inherit",
  });
  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    throw new Error(`Command failed (${exitCode}): ${cmd.join(" ")}`);
  }
}

function bumpVersion(version: string, bump: "patch" | "minor" | "major"): string {
  const [major, minor, patch] = version.split(".").map(Number);
  switch (bump) {
    case "major": return `${major + 1}.0.0`;
    case "minor": return `${major}.${minor + 1}.0`;
    case "patch": return `${major}.${minor}.${patch + 1}`;
  }
}

function pkgDir(name: string): string {
  return resolve(ROOT_DIR, "packages", name);
}

async function readPkgJson(name: string): Promise<any> {
  return Bun.file(resolve(pkgDir(name), "package.json")).json();
}

async function writePkgJson(name: string, data: any): Promise<void> {
  await Bun.write(resolve(pkgDir(name), "package.json"), JSON.stringify(data, null, 2) + "\n");
}

// --- Preflight checks ---

const ALLOWED_BRANCHES = ["main"];

async function preflight() {
  // 1. Dirty working tree
  const status = await run(["git", "status", "--porcelain"]);
  if (status) {
    console.error(c.red("Dirty working tree. Commit or stash changes before publishing:"));
    console.error(status);
    process.exit(1);
  }

  // 2. Branch guard
  const branch = await run(["git", "rev-parse", "--abbrev-ref", "HEAD"]);
  if (!ALLOWED_BRANCHES.includes(branch)) {
    if (ALLOW_BRANCH) {
      console.log(c.yellow(`Publishing from non-standard branch: ${branch} (--allow-branch)`));
    } else {
      console.error(c.red(`Publishing is only allowed from: ${ALLOWED_BRANCHES.join(", ")}`));
      console.error(c.red(`Current branch: ${branch}`));
      console.error(c.red(`Use --allow-branch to override.`));
      process.exit(1);
    }
  }

  // 3. npm auth check
  try {
    await run(["npm", "whoami", "--registry", REGISTRY]);
  } catch {
    console.error(c.red("Not logged into npm. Run `npm login` first."));
    process.exit(1);
  }

  // 4. Local/remote sync
  try {
    await run(["git", "fetch", "origin", branch]);
    const local = await run(["git", "rev-parse", "HEAD"]);
    const remote = await run(["git", "rev-parse", `origin/${branch}`]);
    if (local !== remote) {
      const behind = await run(["git", "rev-list", "--count", `HEAD..origin/${branch}`]);
      if (Number(behind) > 0) {
        console.error(c.red(`Local branch is ${behind} commit(s) behind origin/${branch}. Pull before publishing.`));
        process.exit(1);
      }
      console.log(c.yellow(`Local branch is ahead of origin/${branch} — make sure you've pushed your changes.`));
    }
  } catch {
    console.log(c.yellow("Could not check remote sync (no remote tracking branch?)"));
  }
}

if (!DRY_RUN) {
  await preflight();
  console.log(c.green("Preflight checks passed.\n"));
}

// --- Discover publishable packages and sort by dependency order ---

const SCOPE = "@saswatds/";
const packagesDir = resolve(ROOT_DIR, "packages");

async function discoverPublishOrder(): Promise<{ sorted: string[]; dependentsOf: Map<string, string[]> }> {
  const { readdir } = await import("fs/promises");
  const entries = await readdir(packagesDir, { withFileTypes: true });

  // Collect publishable packages and their workspace deps
  const pkgs = new Map<string, string[]>(); // shortName -> [dep shortNames]

  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    const pkgJsonPath = resolve(packagesDir, entry.name, "package.json");
    if (!(await Bun.file(pkgJsonPath).exists())) continue;

    const pkg = await Bun.file(pkgJsonPath).json();
    if (pkg.private) continue;
    if (!pkg.publishConfig) continue;
    if (!pkg.main && !pkg.exports) continue; // skip non-library packages

    const allDeps = { ...(pkg.dependencies ?? {}), ...(pkg.devDependencies ?? {}) };
    const workspaceDeps = Object.entries(allDeps)
      .filter(([, v]) => (v as string).startsWith("workspace:"))
      .map(([name]) => name.replace(SCOPE, ""));

    pkgs.set(entry.name, workspaceDeps);
  }

  // Topological sort (Kahn's algorithm)
  const inDegree = new Map<string, number>();
  const graph = new Map<string, string[]>(); // dep -> dependents

  for (const name of pkgs.keys()) {
    inDegree.set(name, 0);
    graph.set(name, []);
  }

  for (const [name, deps] of pkgs) {
    for (const dep of deps) {
      if (!pkgs.has(dep)) continue; // external or private dep
      graph.get(dep)!.push(name);
      inDegree.set(name, inDegree.get(name)! + 1);
    }
  }

  const queue = [...pkgs.keys()].filter((n) => inDegree.get(n) === 0);
  const sorted: string[] = [];

  while (queue.length > 0) {
    // Sort the queue alphabetically for deterministic ordering at same depth
    queue.sort();
    const node = queue.shift()!;
    sorted.push(node);
    for (const dependent of graph.get(node)!) {
      const deg = inDegree.get(dependent)! - 1;
      inDegree.set(dependent, deg);
      if (deg === 0) queue.push(dependent);
    }
  }

  if (sorted.length !== pkgs.size) {
    const missing = [...pkgs.keys()].filter((n) => !sorted.includes(n));
    throw new Error(`Circular dependency detected among: ${missing.join(", ")}`);
  }

  return { sorted, dependentsOf: graph };
}

const { sorted: PUBLISH_ORDER, dependentsOf } = await discoverPublishOrder();
console.log(c.blue(`Discovered publish order: ${PUBLISH_ORDER.join(" → ")}`));

/** Given a set of directly affected packages, expand to include all transitive dependents. */
function expandWithDependents(direct: string[]): string[] {
  const expanded = new Set(direct);
  const queue = [...direct];
  while (queue.length > 0) {
    const pkg = queue.shift()!;
    for (const dep of dependentsOf.get(pkg) ?? []) {
      if (!expanded.has(dep)) {
        expanded.add(dep);
        queue.push(dep);
        console.log(c.blue(`  Adding transitive dependent: ${dep} (depends on ${pkg})`));
      }
    }
  }
  return [...expanded];
}

// --- Step 1: Find last publish tag ---

let baseRef: string | null = null;
try {
  baseRef = await run(["git", "describe", "--tags", "--abbrev=0", "--match", "publish-*"]);
  console.log(c.blue(`Using base ref from tag: ${baseRef}`));
} catch {
  console.log(c.yellow("No previous publish tag found, will check all packages"));
}

// --- Step 1b: Determine bump type from conventional commits ---

async function inferBumpFromCommits(since: string | null): Promise<"patch" | "minor" | "major"> {
  const range = since ? `${since}..HEAD` : "HEAD";
  // %s = subject line, %b = body, separated by a delimiter
  const log = await run(["git", "log", range, "--pretty=format:%s%n%b%n---COMMIT---"]);
  const commits = log.split("---COMMIT---").map((s) => s.trim()).filter(Boolean);

  let bump: "patch" | "minor" | "major" = "patch";

  for (const commit of commits) {
    const [subject, ...bodyLines] = commit.split("\n");
    const body = bodyLines.join("\n");

    // BREAKING CHANGE in body/footer or `!` after type/scope → major
    if (body.includes("BREAKING CHANGE") || body.includes("BREAKING-CHANGE") || /^\w+(\(.+\))?!:/.test(subject)) {
      return "major"; // short-circuit, can't go higher
    }

    // feat → minor
    if (/^feat(\(.+\))?:/.test(subject)) {
      bump = "minor";
    }
    // fix, chore, docs, refactor, etc. → patch (already the default)
  }

  return bump;
}

let BUMP_TYPE: "patch" | "minor" | "major";

if (BUMP_OVERRIDE) {
  BUMP_TYPE = BUMP_OVERRIDE;
  console.log(c.blue(`Bump type (manual override): ${BUMP_TYPE}`));
} else {
  BUMP_TYPE = await inferBumpFromCommits(baseRef);
  console.log(c.blue(`Bump type (from conventional commits): ${BUMP_TYPE}`));
}

// --- Step 2: Detect affected packages via Moon ---

let affected: string[] = [];

if (baseRef) {
  try {
    const out = await run(["moon", "query", "projects", "--affected", "--json"], {
      env: { MOON_BASE: baseRef },
    });
    const data = JSON.parse(out);
    const projects: string[] = (data.projects ?? [])
      .map((p: any) => p.id ?? p)
      .filter((id: string) => id.startsWith("astro-"));
    affected = projects;
  } catch (e) {
    console.log(c.yellow(`Moon query failed, falling back to all packages: ${e}`));
  }
}

if (affected.length === 0) {
  console.log(c.yellow("No affected packages detected, checking all publishable packages"));
  affected = [...PUBLISH_ORDER];
} else {
  // Expand to include transitive dependents
  console.log(c.blue(`Directly affected: ${affected.join(", ")}`));
  affected = expandWithDependents(affected);
}

// --- Step 3: Filter and order ---

const packagesToPublish: string[] = [];

for (const name of PUBLISH_ORDER) {
  if (!affected.includes(name)) continue;
  const dir = pkgDir(name);
  if (!(await Bun.file(resolve(dir, "package.json")).exists())) continue;
  const pkg = await readPkgJson(name);
  if (pkg.private) continue;
  packagesToPublish.push(name);
}

if (packagesToPublish.length === 0) {
  console.log(c.green("No packages need publishing."));
  process.exit(0);
}

// --- Step 4: Check registry versions and show plan ---

async function getRegistryVersion(pkgName: string): Promise<string | null> {
  try {
    const out = await run(["npm", "view", pkgName, "version", "--registry", REGISTRY]);
    return out || null;
  } catch {
    return null; // not yet published
  }
}

function compareVersions(a: string, b: string): number {
  const [aMaj, aMin, aPat] = a.split(".").map(Number);
  const [bMaj, bMin, bPat] = b.split(".").map(Number);
  return aMaj - bMaj || aMin - bMin || aPat - bPat;
}

console.log("");
console.log(c.yellow("Packages to publish (in order):"));

const versionMap = new Map<string, { local: string; registry: string | null; current: string; next: string }>();

for (const name of packagesToPublish) {
  const pkg = await readPkgJson(name);
  const local = pkg.version;
  const registry = await getRegistryVersion(pkg.name);
  // Use whichever is higher as the base for bumping
  const current = registry && compareVersions(registry, local) > 0 ? registry : local;
  const next = bumpVersion(current, BUMP_TYPE);
  versionMap.set(name, { local, registry, current, next });
  const registryInfo = registry ? (registry !== local ? c.yellow(` (registry: ${registry})`) : ` (registry: ${registry})`) : " (not yet published)";
  console.log(`  ${c.green(name)}: ${current} → ${next}${registryInfo}`);
}

if (DRY_RUN) {
  console.log("");
  console.log(c.yellow("Dry run — no changes made"));
  process.exit(0);
}

// --- Step 5: Confirm ---

if (!SKIP_CONFIRM) {
  process.stdout.write("\nProceed with version bump and publish? [y/N] ");
  const buf = Buffer.alloc(8);
  const n = require("fs").readSync(0, buf, 0, buf.length, null);
  const answer = buf.toString("utf8", 0, n).trim();
  if (!/^[Yy]$/.test(answer)) {
    console.log(c.yellow("Aborted."));
    process.exit(0);
  }
}

// --- Step 6: Bump versions ---

console.log("");
console.log(c.yellow("Bumping versions..."));

for (const name of packagesToPublish) {
  const { local, current, next } = versionMap.get(name)!;
  const pkg = await readPkgJson(name);
  pkg.version = next;
  await writePkgJson(name, pkg);
  const note = current !== local ? c.yellow(` (was ${local} locally, ${current} on registry)`) : "";
  console.log(`  ${c.green(name)}: ${current} → ${next}${note}`);
}

// --- Step 7: Resolve workspace:* → real versions ---

console.log("");
console.log(c.yellow("Resolving workspace references..."));

// Build a map of all package versions (including those not being published)
const allVersions = new Map<string, string>();
for (const name of PUBLISH_ORDER) {
  try {
    const pkg = await readPkgJson(name);
    allVersions.set(`@saswatds/${name}`, pkg.version);
  } catch {}
}

function resolveDeps(deps: Record<string, string> | undefined): Record<string, string> | undefined {
  if (!deps) return deps;
  const resolved: Record<string, string> = {};
  for (const [name, version] of Object.entries(deps)) {
    if (version.startsWith("workspace:") && allVersions.has(name)) {
      resolved[name] = allVersions.get(name)!;
    } else {
      resolved[name] = version;
    }
  }
  return resolved;
}

const originalPkgs = new Map<string, any>();

for (const name of packagesToPublish) {
  const pkg = await readPkgJson(name);
  // Save original for restore later
  originalPkgs.set(name, structuredClone(pkg));
  pkg.dependencies = resolveDeps(pkg.dependencies);
  pkg.devDependencies = resolveDeps(pkg.devDependencies);
  await writePkgJson(name, pkg);
  console.log(`  ${c.green(name)}: resolved workspace:* → actual versions`);
}

// --- Step 8: Build ---

console.log("");
console.log(c.yellow("Building all packages..."));
await runPassthrough(["bun", "run", "build"]);

// --- Step 9: Publish ---

console.log("");
console.log(c.yellow("Publishing packages..."));

for (const name of packagesToPublish) {
  const pkg = await readPkgJson(name);
  console.log(`\n${c.green(`[${name}]`)} Publishing ${pkg.name}@${pkg.version}...`);
  await runPassthrough(["bun", "publish", "--access", "restricted"], { cwd: pkgDir(name) });
  console.log(c.green(`[${name}] Published successfully`));
}

// --- Step 10: Restore workspace:* refs ---

console.log("");
console.log(c.yellow("Restoring workspace references..."));

for (const name of packagesToPublish) {
  const pkg = await readPkgJson(name);
  // Restore workspace:* for any @saswatds/ dep that had a resolved version
  const restoreDeps = (deps: Record<string, string> | undefined) => {
    if (!deps) return deps;
    const restored: Record<string, string> = {};
    for (const [depName, ver] of Object.entries(deps)) {
      if (depName.startsWith("@saswatds/") && /^\d+\.\d+\.\d+/.test(ver)) {
        restored[depName] = "workspace:*";
      } else {
        restored[depName] = ver;
      }
    }
    return restored;
  };
  pkg.dependencies = restoreDeps(pkg.dependencies);
  pkg.devDependencies = restoreDeps(pkg.devDependencies);
  await writePkgJson(name, pkg);
  console.log(`  ${c.green(name)}: restored workspace:*`);
}

// --- Step 11: bun install to update lockfile ---

console.log("");
console.log(c.yellow("Updating lockfile..."));
await runPassthrough(["bun", "install"]);

// --- Step 12: Commit & tag ---

let tag = `publish-${new Date().toISOString().replace(/[-:T.]/g, "").slice(0, 14)}`;

// Git tag collision check
try {
  await run(["git", "rev-parse", tag]);
  // Tag exists — append a suffix to make it unique
  console.log(c.yellow(`Tag ${tag} already exists, appending suffix...`));
  tag = `${tag}-${Math.random().toString(36).slice(2, 6)}`;
} catch {
  // Tag doesn't exist — good
}

const publishedList = packagesToPublish
  .map((name) => `- @saswatds/${name}@${versionMap.get(name)!.next}`)
  .join("\n");

await run(["git", "add", "packages/*/package.json", "bun.lock"]);
await run([
  "git", "commit", "-m",
  `chore: publish packages\n\nPackages published:\n${publishedList}`,
]);
await run(["git", "tag", tag]);

// --- Step 13: Post-publish verification ---

console.log("");
console.log(c.yellow("Verifying published versions..."));

let allVerified = true;
for (const name of packagesToPublish) {
  const { next } = versionMap.get(name)!;
  const fullName = `${SCOPE}${name}`;
  // Retry a few times since registry can be slow to propagate
  let verified = false;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const published = await run(["npm", "view", `${fullName}@${next}`, "version", "--registry", REGISTRY]);
      if (published === next) {
        console.log(`  ${c.green(name)}: ${next} verified on registry`);
        verified = true;
        break;
      }
    } catch {}
    if (attempt < 2) await new Promise((r) => setTimeout(r, 2000));
  }
  if (!verified) {
    console.log(`  ${c.red(name)}: ${next} NOT found on registry — may need manual verification`);
    allVerified = false;
  }
}

console.log("");
if (allVerified) {
  console.log(c.green("All packages published and verified!"));
} else {
  console.log(c.yellow("Some packages could not be verified — check the registry manually."));
}
console.log(c.blue(`Created tag: ${tag}`));
console.log(c.yellow("Don't forget to push: git push && git push --tags"));
