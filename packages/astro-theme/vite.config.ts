import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { astroThemeColors } from "./src/vite-plugin";

export default defineConfig({
  plugins: [astroThemeColors(), react(), tailwindcss()],
});
