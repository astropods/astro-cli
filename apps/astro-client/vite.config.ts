import { reactRouter } from "@react-router/dev/vite";
import { defineConfig, loadEnv } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { astroThemeColors } from "astro-theme/plugin";
import fs from "fs";
import path from "path";

// Local development domain (must match what's in /etc/hosts)
const LOCAL_DOMAIN = "local.astropods.ai";

// Check for local HTTPS certificates
function getHttpsConfig() {
  const certDir = path.resolve(__dirname, ".certs");
  const certPath = path.join(certDir, `${LOCAL_DOMAIN}.pem`);
  const keyPath = path.join(certDir, `${LOCAL_DOMAIN}-key.pem`);

  if (fs.existsSync(certPath) && fs.existsSync(keyPath)) {
    return {
      cert: fs.readFileSync(certPath),
      key: fs.readFileSync(keyPath),
    };
  }
  return undefined;
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const apiTarget = env.VITE_API_URL || "http://localhost:8080";
  const httpsConfig = getHttpsConfig();

  // Use local domain when HTTPS is configured (for same-site cookie sharing)
  const useLocalDomain = !!httpsConfig;

  return {
    plugins: [
      astroThemeColors(),
      tailwindcss(),
      !process.env.STORYBOOK && reactRouter(),
    ].filter(Boolean),
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      // When certs are present, bind to the local domain for same-site cookies
      host: useLocalDomain ? LOCAL_DOMAIN : "localhost",
      https: httpsConfig,
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
      },
    },
  };
});
