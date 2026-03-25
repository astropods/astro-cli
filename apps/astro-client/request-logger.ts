import { getChildLogger } from "./logger";

const log = getChildLogger("http");

type FetchHandler = (request: Request) => Response | Promise<Response>;

const SKIP_PATHS = ["/healthz", "/readyz", "/favicon.ico"];

function classify(pathname: string): "proxy" | "static" | "ssr" {
  if (["/auth", "/api", "/download", "/install", "/schema"].some((p) => pathname.startsWith(p))) {
    return "proxy";
  }
  if (pathname.startsWith("/assets/") || pathname.match(/\.\w{2,5}$/)) {
    return "static";
  }
  return "ssr";
}

/**
 * Morgan-style request logging middleware for Bun.serve.
 * Wraps a fetch handler and logs every request with method, path, status, and duration.
 * Static asset requests are logged at debug level, proxy at debug, SSR at info/warn/error.
 */
export function withLogging(handler: FetchHandler): FetchHandler {
  return async (request: Request) => {
    const start = performance.now();
    const url = new URL(request.url);
    const { pathname } = url;

    if (SKIP_PATHS.includes(pathname)) {
      return handler(request);
    }

    const kind = classify(pathname);

    let res: Response;
    try {
      res = await handler(request);
    } catch (err) {
      const duration = Math.round(performance.now() - start);
      log.error("Request failed: {method} {path} ({duration}ms) error={error}", {
        method: request.method, path: pathname, error: err instanceof Error ? err.message : String(err), duration,
      });
      throw err;
    }

    const duration = Math.round(performance.now() - start);
    const props = { method: request.method, path: pathname, status: res.status, duration };

    if (kind === "static") {
      log.debug("{method} {path} {status} ({duration}ms)", props);
    } else if (kind === "proxy") {
      log.debug("{method} {path} -> proxy {status} ({duration}ms)", props);
    } else if (res.status >= 500) {
      log.error("{method} {path} {status} ({duration}ms)", props);
    } else if (res.status >= 400) {
      log.warn("{method} {path} {status} ({duration}ms)", props);
    } else {
      log.info("{method} {path} {status} ({duration}ms)", props);
    }

    return res;
  };
}
