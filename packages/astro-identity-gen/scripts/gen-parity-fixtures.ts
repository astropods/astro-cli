/**
 * One-shot: captures parity fixtures from the TS reference implementation and
 * writes them to the Go package's testdata directory. Run after any intentional
 * change to the TS rng/generate logic (rare — TS is being deprecated).
 *
 *   bun run scripts/gen-parity-fixtures.ts
 */

import { mkdirSync, writeFileSync } from "fs";
import { resolve } from "path";
import { hash, createRng } from "../src/rng";
import { generateIdentityWithChoices } from "../src/generate";

const GO_TESTDATA = resolve(
  import.meta.dir,
  "../../../apps/astro-server/internal/identitygen/testdata",
);

// Fixed seed list: mix of empty, ASCII, unicode, emoji, long, slashed handles.
const SEEDS = [
  "",
  "a",
  "alice",
  "bob",
  "test-seed",
  "stable",
  "account/agent-1",
  "postman/research-bot",
  "this-is-a-longer-seed-value-for-diversity",
  "42",
  "hello world",
  "path/to/thing",
  "日本語",
  "你好",
  "😀-unicode",
  "🚀🚀🚀",
  "mixed-ASCII-and-日本",
  "x7y8z9",
  "abc123def",
  "a/b/c/d/e",
] as const;

// Sizes to capture choices at. Size affects no decisions (only the output SVG),
// but we record it so Go tests can pass it through.
const SIZES = [64, 128, 256] as const;

type HashRngRow = {
  seed: string;
  hash: number;
  rng100: string[]; // float64 bit patterns as hex strings for exact equality
};

type ChoicesRow = {
  seed: string;
  size: number;
  choices: ReturnType<typeof generateIdentityWithChoices>["choices"];
};

// Convert a float64 to its IEEE 754 hex bit pattern so JSON round-trips exactly.
function floatToBits(v: number): string {
  const buf = new ArrayBuffer(8);
  new Float64Array(buf)[0] = v;
  const u = new Uint8Array(buf);
  // Little-endian on all supported platforms; match Go's math.Float64bits (big-endian hex).
  let hex = "";
  for (let i = 7; i >= 0; i--) {
    hex += u[i].toString(16).padStart(2, "0");
  }
  return hex;
}

const hashRng: HashRngRow[] = SEEDS.map((seed) => {
  const h = hash(seed);
  const rng = createRng(h);
  const rng100: string[] = [];
  for (let i = 0; i < 100; i++) {
    rng100.push(floatToBits(rng()));
  }
  return { seed, hash: h, rng100 };
});

const choices: ChoicesRow[] = [];
for (const seed of SEEDS) {
  for (const size of SIZES) {
    const { choices: c } = generateIdentityWithChoices({ seed, size });
    choices.push({ seed, size, choices: c });
  }
}

mkdirSync(GO_TESTDATA, { recursive: true });
writeFileSync(
  `${GO_TESTDATA}/hash_rng.json`,
  JSON.stringify(hashRng, null, 2) + "\n",
);
writeFileSync(
  `${GO_TESTDATA}/choices.json`,
  JSON.stringify(choices, null, 2) + "\n",
);

console.log(
  `Wrote ${hashRng.length} hash/rng rows and ${choices.length} choice rows to ${GO_TESTDATA}`,
);
