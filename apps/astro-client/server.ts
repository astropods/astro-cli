import path from "path";
import { createRequestHandler, type ServerBuild } from "react-router";

const serverBuildPath = "./build/server/index.js";
const build: ServerBuild = await import(serverBuildPath);
const mode = process.env.NODE_ENV || "production";
const handler = createRequestHandler(build, mode);

const port = Number(process.env.PORT) || 3000;
const API_URL = process.env.API_URL || "http://localhost:8080";
const CLIENT_BUILD_DIR = path.resolve("./build/client");
const PROXY_PREFIXES = ["/auth", "/api", "/download", "/install"];

Bun.serve({
  port,
  async fetch(request) {
    const url = new URL(request.url);

    // Proxy API, auth, and other backend routes to the Go server
    if (PROXY_PREFIXES.some((prefix) => url.pathname.startsWith(prefix))) {
      const target = new URL(url.pathname + url.search, API_URL);
      const headers = new Headers(request.headers);
      headers.set("host", new URL(API_URL).host);
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
    return handler(request);
  },
});

console.log(`astro-client listening on :${port}`);
