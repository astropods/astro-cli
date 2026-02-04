import type { AstroAgent } from "@saswatds/astro-agent";
import {
  createApiServer,
  type PlaygroundApiServer,
  type PlaygroundApiServerOptions,
} from "./server";
import {
  createViteServer,
  type PlaygroundViteServer,
  type ViteServerOptions,
} from "./vite";

export type StartPlaygroundOptions = {
  /**
   * A record of agents keyed by their ID.
   * The ID is used in the API routes and UI to identify the agent.
   */
  agents: Record<string, AstroAgent>;
  /**
   * Port to run the API server on.
   * @default 3001
   */
  apiPort?: number;
  /**
   * Port to run the Vite dev server on.
   * @default 5173
   */
  vitePort?: number;
  /**
   * Whether to open the browser automatically.
   * @default true
   */
  open?: boolean;
};

export type PlaygroundInstance = {
  /**
   * The API server instance.
   */
  apiServer: PlaygroundApiServer;
  /**
   * The Vite dev server instance.
   */
  viteServer: PlaygroundViteServer;
  /**
   * Stops both servers.
   */
  stop: () => Promise<void>;
};

/**
 * Starts the Astro Playground with the provided agents.
 *
 * This starts both:
 * 1. The API server that handles agent interactions
 * 2. The Vite dev server that serves the playground UI
 *
 * @example
 * ```typescript
 * import { startPlayground } from 'astro-playground';
 * import { myAgent } from './my-agents';
 *
 * const playground = await startPlayground({
 *   agents: {
 *     'my-agent': myAgent,
 *   },
 * });
 *
 * // Later, to stop the servers:
 * await playground.stop();
 * ```
 */
export async function startPlayground(
  options: StartPlaygroundOptions
): Promise<PlaygroundInstance> {
  const { agents, apiPort = 3001, vitePort = 5173, open = true } = options;

  console.log("\n🚀 Starting Astro Playground...\n");

  // Start the API server
  const apiServer = createApiServer({
    agents,
    port: apiPort,
  });

  // Start the Vite dev server
  const viteServer = await createViteServer({
    port: vitePort,
    open,
  });

  console.log("\n✨ Playground is ready!\n");

  return {
    apiServer,
    viteServer,
    stop: async () => {
      apiServer.stop();
      await viteServer.stop();
      console.log("\n👋 Playground stopped.\n");
    },
  };
}

// Re-export types and individual server creators for advanced usage
export { createApiServer, createViteServer };
export type {
  PlaygroundApiServer,
  PlaygroundApiServerOptions,
  PlaygroundViteServer,
  ViteServerOptions,
};
