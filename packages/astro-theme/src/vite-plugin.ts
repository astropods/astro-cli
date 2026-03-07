/**
 * Vite plugin that regenerates dist/colors.css and dist/semantic.css
 * when colors.ts or semantic.ts change.
 *
 * Usage:
 *   import { astroThemeColors } from "astro-theme/plugin";
 *   plugins: [astroThemeColors(), ...]
 */
import type { Plugin } from "vite";
import { execSync } from "child_process";
import { dirname, resolve } from "path";
import { fileURLToPath } from "url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const PACKAGE_ROOT = resolve(__dirname, "..");
const COLORS_SRC = resolve(__dirname, "colors.ts");
const SEMANTIC_SRC = resolve(__dirname, "semantic.ts");
const TYPOGRAPHY_SRC = resolve(__dirname, "typography.ts");

function buildCSS() {
  execSync("bun run src/build-css.ts", { cwd: PACKAGE_ROOT, stdio: "inherit" });
}

export function astroThemeColors(): Plugin {
  return {
    name: "astro-theme-colors",

    buildStart() {
      buildCSS();
    },

    configureServer(server) {
      server.watcher.add(COLORS_SRC);
      server.watcher.add(SEMANTIC_SRC);
      server.watcher.add(TYPOGRAPHY_SRC);
      server.watcher.on("change", (path) => {
        if (path === COLORS_SRC || path === SEMANTIC_SRC || path === TYPOGRAPHY_SRC) {
          buildCSS();
        }
      });
    },
  };
}
