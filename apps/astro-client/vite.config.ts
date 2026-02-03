import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.VITE_API_URL || 'http://localhost:8080';

  return {
    plugins: [react(), tailwindcss()],
    server: {
      proxy: {
        // Proxy API requests to the backend
        '/api': {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
        // Proxy auth requests to the backend
        '/auth': {
          target: apiTarget,
          changeOrigin: true,
          secure: true,
        },
      },
    },
  };
});
