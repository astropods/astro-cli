/**
 * Generates dist/colors.css and dist/semantic.css from TypeScript sources.
 * Run: bun run src/build-css.ts
 */
import { writeFileSync, mkdirSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { palettes } from "./colors";
import { lightTheme, darkTheme } from "./semantic";

const __filename = fileURLToPath(import.meta.url);
const packageRoot = dirname(dirname(__filename));
const outDir = join(packageRoot, "dist");

const steps = [25, 50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950] as const;

function generateColorsCSS(): string {
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

function generateSemanticCSS(): string {
  const lines: string[] = [
    "/* Generated from semantic.ts — do not edit manually */",
    "",
    ":root {",
    "  --radius: 0.75rem;",
  ];

  for (const [key, value] of Object.entries(lightTheme)) {
    lines.push(`  --${key}: ${value};`);
  }

  lines.push("}", "", ".dark {");

  for (const [key, value] of Object.entries(darkTheme)) {
    lines.push(`  --${key}: ${value};`);
  }

  lines.push("}");
  return lines.join("\n") + "\n";
}

mkdirSync(outDir, { recursive: true });

const colorsPath = join(outDir, "colors.css");
writeFileSync(colorsPath, generateColorsCSS());
console.log(`Generated ${colorsPath}`);

const semanticPath = join(outDir, "semantic.css");
writeFileSync(semanticPath, generateSemanticCSS());
console.log(`Generated ${semanticPath}`);
