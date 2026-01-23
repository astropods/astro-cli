import { createServer, type ViteDevServer } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "url";
import { dirname, resolve } from "path";

export type ViteServerOptions = {
  /**
   * Port to run the Vite dev server on.
   * @default 5173
   */
  port?: number;
  /**
   * Whether to open the browser automatically.
   * @default false
   */
  open?: boolean;
};

export type PlaygroundViteServer = {
  port: number;
  stop: () => Promise<void>;
};

/**
 * Creates and starts the Vite dev server for the playground frontend.
 */
export async function createViteServer(
  options: ViteServerOptions = {}
): Promise<PlaygroundViteServer> {
  const { port = 5173, open = false } = options;

  // Get the package root directory
  const __filename = fileURLToPath(import.meta.url);
  const __dirname = dirname(__filename);
  const packageRoot = resolve(__dirname, "..");

  const server: ViteDevServer = await createServer({
    root: packageRoot,
    plugins: [react(), tailwindcss()],
    server: {
      port,
      open,
    },
    configFile: false,
  });

  await server.listen();

  const resolvedPort = server.config.server.port ?? port;

  console.log(
    `🎨 Astro Playground UI running at http://localhost:${resolvedPort}`
  );

  return {
    port: resolvedPort,
    stop: () => server.close(),
  };
}
