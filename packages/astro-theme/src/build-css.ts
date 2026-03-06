/**
 * Generates dist/colors.css from the palette definitions in colors.ts.
 * Run: bun run src/build-css.ts
 */
import { writeFileSync, mkdirSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { palettes } from "./colors";

const __filename = fileURLToPath(import.meta.url);
const packageRoot = dirname(dirname(__filename));
const outDir = join(packageRoot, "dist");

const steps = [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;

function generateCSS(): string {
  const lines: string[] = [
    "/* Generated from colors.ts — do not edit manually */",
    "@theme static {",
    "  --color-*: initial;",
    "",
    "  --color-black: #000;",
    "  --color-white: #fff;",
  ];

  for (const [name, scale] of Object.entries(palettes)) {
    lines.push("");
    for (const step of steps) {
      lines.push(`  --color-${name}-${step}: ${scale[step]};`);
    }
  }

  lines.push("}");
  return lines.join("\n") + "\n";
}

mkdirSync(outDir, { recursive: true });
const outPath = join(outDir, "colors.css");
writeFileSync(outPath, generateCSS());
console.log(`Generated ${outPath}`);
