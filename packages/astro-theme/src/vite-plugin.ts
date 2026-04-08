/**
 * Vite plugin that regenerates dist/colors.css and dist/semantic.css
 * when colors.ts or semantic.ts change.
 *
 * Usage:
 *   import { astroThemeColors } from "@astropods/theme/plugin";
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

const THEME_SOURCES = new Set([COLORS_SRC, SEMANTIC_SRC, TYPOGRAPHY_SRC]);

function buildCSS() {
  execSync("bun run src/build-css.ts", { cwd: PACKAGE_ROOT, stdio: "inherit" });
}

export function astroThemeColors(): Plugin {
  let rebuildTimer: ReturnType<typeof setTimeout> | null = null;

  return {
    name: "astro-theme-colors",

    buildStart() {
      buildCSS();
    },

    configureServer(server) {
      for (const src of THEME_SOURCES) server.watcher.add(src);
      server.watcher.on("change", (path) => {
        if (!THEME_SOURCES.has(path)) return;
        if (rebuildTimer) clearTimeout(rebuildTimer);
        rebuildTimer = setTimeout(() => {
          rebuildTimer = null;
          buildCSS();
        }, 80);
      });
    },
  };
}
