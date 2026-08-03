import { reactRouter } from "@react-router/dev/vite";
import { defineConfig, loadEnv } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { astroThemeColors } from "@astropods/theme/plugin";
import fs from "fs";
import path from "path";

// Vite plugin: copies blueprint-jellybean font files into the SSR output directory
// so that import.meta.url-relative font resolution works at runtime.
//
// React Router v7 runs two Vite build passes: client (outDir=build/client) and
// server (outDir=build/server). We only want to run during the server pass, and
// the dest is outDir/assets/fonts (NOT outDir/server/assets/fonts — outDir already
// IS build/server). config.build.ssr is truthy only during the server pass.
function copyBlueprintFonts(): import("vite").Plugin {
  let resolvedConfig: import("vite").ResolvedConfig;
  return {
    name: "copy-blueprint-fonts",
    apply: "build",
    configResolved(config) {
      resolvedConfig = config;
    },
    closeBundle() {
if (!resolvedConfig.build.ssr) return;
      const src  = path.resolve(__dirname, "../../packages/blueprint-jellybean/fonts");
      const dest = path.resolve(resolvedConfig.build.outDir, "assets/fonts");
      if (!fs.existsSync(src)) return;
      fs.mkdirSync(dest, { recursive: true });
      for (const f of fs.readdirSync(src)) {
        fs.copyFileSync(path.join(src, f), path.join(dest, f));
      }
    },
  };
}
// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const apiTarget = env.VITE_API_URL || "http://localhost:8080";

  return {
    plugins: [
      astroThemeColors(),
      tailwindcss(),
      !process.env.STORYBOOK && reactRouter(),
      copyBlueprintFonts(),
    ].filter(Boolean),
    // Workspace package `astro-trading-card` imports `uqr`; SSR must bundle them so
    // Node resolves from the Vite graph (avoids "Cannot find module 'uqr'" in dev).
    optimizeDeps: {
      include: ["uqr", "astro-trading-card"],
    },
    ssr: {
      noExternal: ["astro-trading-card", "uqr"],
    },
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      // Bind on all interfaces. Node 18+ resolves "localhost" as IPv6 only,
      // which makes the dev server unreachable from local-mode k8s pods that
      // dial via host.docker.internal (an IPv4 address from the pod's POV).
      host: true,
      // Allow messaging pods running in local-mode k8s to reach the dev server
      // via Docker Desktop's host alias; Vite otherwise rejects the Host header.
      allowedHosts: ["host.docker.internal"],
      proxy: {
        // Proxy API requests to the backend
        "/api": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy CLI binary download to the backend (Dev page links and curl use /download/*)
        "/download": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy CLI install script to the backend
        "/install": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy auth endpoints to the backend
        "/auth": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy webhook endpoints to the backend (e.g. GitHub webhook delivery)
        "/webhooks": {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
      },
    },
  };
});
