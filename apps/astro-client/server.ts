import path from "path";
import { createRequestHandler, type ServerBuild } from "react-router";

const serverBuildPath = "./build/server/index.js";
const build: ServerBuild = await import(serverBuildPath);
const mode = process.env.NODE_ENV || "production";
const handler = createRequestHandler(build, mode);
const SUPPRESS_E2E_RENDER_ABORT_LOGS = process.env.E2E_SUPPRESS_ABORT_LOGS === "1";
const RENDER_ABORT_WITHOUT_REASON = "The render was aborted by the server without a reason.";

const port = Number(process.env.PORT) || 3000;
const API_URL = process.env.API_URL || "http://localhost:8080";
const CLIENT_BUILD_DIR = path.resolve("./build/client");
const PROXY_PREFIXES = ["/auth", "/api", "/download", "/install", "/schema"];

const isBenignRenderAbort = (value: unknown): boolean => {
  if (value instanceof Error) return value.message.includes(RENDER_ABORT_WITHOUT_REASON);
  return String(value).includes(RENDER_ABORT_WITHOUT_REASON);
};

if (SUPPRESS_E2E_RENDER_ABORT_LOGS) {
  const originalConsoleError = console.error.bind(console);
  /*
   * React can log this abort directly via console.error during stream cancellation.
   * Filter only the known abort signal in E2E mode to keep Playwright output clean.
   */
  console.error = (...args: unknown[]) => {
    if (args.some((arg) => isBenignRenderAbort(arg))) return;
    originalConsoleError(...args);
  };

  /*
   * Playwright E2E can intentionally interrupt SSR streams during navigation/teardown.
   * React reports this as "render was aborted ... without a reason", which is expected
   * in this test mode and creates noisy logs without affecting correctness.
   * We suppress only this exact signal in E2E mode, while still failing fast on anything else.
   */
  process.on("unhandledRejection", (reason) => {
    if (isBenignRenderAbort(reason)) return;
    console.error(reason);
    process.exit(1);
  });

  process.on("uncaughtException", (error) => {
    if (isBenignRenderAbort(error)) return;
    console.error(error);
    process.exit(1);
  });
}

Bun.serve({
  port,
  async fetch(request) {
    const url = new URL(request.url);

    // Proxy API, auth, and other backend routes to the Go server
    if (PROXY_PREFIXES.some((prefix) => url.pathname.startsWith(prefix))) {
      const target = new URL(url.pathname + url.search, API_URL);
      const headers = new Headers(request.headers);
      return fetch(target, {
        method: request.method,
        headers,
        body: request.body,
        redirect: "manual",
      });
    }

    // Resolve and verify the path stays within the client build directory
    const resolved = path.resolve(CLIENT_BUILD_DIR, "." + url.pathname);
    if (
      resolved.startsWith(CLIENT_BUILD_DIR + path.sep) ||
      resolved === CLIENT_BUILD_DIR
    ) {
      const staticFile = Bun.file(resolved);
      if (await staticFile.exists()) {
        const headers: Record<string, string> = {};

        // Vite hashed filenames are immutable; everything else gets a short cache
        if (url.pathname.startsWith("/assets/")) {
          headers["Cache-Control"] =
            "public, max-age=31536000, immutable";
        } else {
          headers["Cache-Control"] = "public, max-age=3600";
        }

        return new Response(staticFile, { headers });
      }
    }

    // SSR for everything else
    try {
      return await handler(request);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      if (SUPPRESS_E2E_RENDER_ABORT_LOGS && message.includes(RENDER_ABORT_WITHOUT_REASON)) {
        return new Response(null, { status: 204 });
      }
      throw err;
    }
  },
});

console.log(`astro-client listening on :${port}`);
