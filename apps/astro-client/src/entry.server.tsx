import type { EntryContext } from "react-router";
import { ServerRouter } from "react-router";
import { isbot } from "isbot";
import { renderToReadableStream } from "react-dom/server";
import { getChildLogger } from "./lib/logger";

const log = getChildLogger("ssr");

export default async function handleRequest(
  request: Request,
  responseStatusCode: number,
  responseHeaders: Headers,
  routerContext: EntryContext,
) {
  const body = await renderToReadableStream(
    <ServerRouter context={routerContext} url={request.url} />,
    {
      onError(error: unknown) {
        log.error("SSR render error: {url} {error}", { error, url: request.url });
        responseStatusCode = 500;
      },
    },
  );

  // Await the full stream for bots so they get complete HTML
  if (isbot(request.headers.get("user-agent") || "")) {
    await body.allReady;
  }

  responseHeaders.set("Content-Type", "text/html; charset=utf-8");

  return new Response(body, {
    headers: responseHeaders,
    status: responseStatusCode,
  });
}
