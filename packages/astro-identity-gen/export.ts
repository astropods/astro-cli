import { mkdirSync, writeFileSync } from "fs";
import { generateIdentity } from "./src/index";

const outDir = import.meta.dir + "/build";
const count = 100;
const size = 128;

mkdirSync(outDir, { recursive: true });

for (let i = 0; i < count; i++) {
  const seed = Math.random().toString(36).slice(2);
  const svg = generateIdentity({ seed, size });
  writeFileSync(`${outDir}/${seed}.svg`, svg);
}

console.log(`Exported ${count} identities to ${outDir}`);
