import { defineConfig } from "vite";
import path from "node:path";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { brandIconsApi } from "./dev/server/plugin";

const REPO_ROOT = path.resolve(__dirname, "../..");

export default defineConfig({
  root: __dirname,
  plugins: [react(), tailwindcss(), brandIconsApi()],
  server: {
    port: 5180,
    open: false,
    // Saving an icon writes SVGs into sources/ and assets/integrations/
    // AND rewrites icons.json (manifest upsert). Vite's HMR watcher would
    // otherwise treat those changes as module updates and force a full
    // page reload, which resets the React app state (Source tab, chat
    // history, etc.). Keep those paths out of the watcher — the UI
    // fetches SVGs over HTTP with a cache-buster and re-fetches the
    // manifest via /api/icons when refreshKey bumps.
    watch: {
      ignored: [
        path.join(__dirname, "sources/**"),
        path.join(__dirname, "icons.json"),
        path.join(REPO_ROOT, "assets/integrations/**"),
      ],
    },
  },
  build: {
    outDir: "dev-dist",
  },
});
