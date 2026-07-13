import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { astroThemeColors } from "@astropods/theme/plugin";
import path from "path";

// Separate, SSR-free build of the chat experience that the astro CLI embeds and
// serves locally. It reuses the same source as the deployed app (src/pages/chat
// + src/components/chat + shared hooks/lib) via the chat-embed entry, so the two
// can never visually drift. The output (chat-embed-dist/) is copied into
// apps/astro-cli/internal/chatui/webdist during the CLI release build (named
// webdist, not dist, because the repo-root .gitignore ignores every "dist" dir).
//
// Deliberately omits the React Router (@react-router/dev) plugin: this is a
// plain SPA, not the framework/SSR build that `react-router build` produces.
export default defineConfig({
  plugins: [astroThemeColors(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "chat-embed-dist",
    emptyOutDir: true,
    rollupOptions: {
      input: path.resolve(__dirname, "chat-embed.html"),
    },
  },
});
