import type { EntryContext } from "react-router";
import { ServerRouter } from "react-router";
import { renderToReadableStream } from "react-dom/server";
import { getChildLogger } from "../logger";

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

  // Streaming the response truncates large payloads in the Bun+ALB+CloudFront
  // pipeline for browser UAs: any account whose loader response is large enough
  // for the turbo-stream encoder to yield to main triggers a Suspense in the
  // SSR data-streaming Suspense boundary, and the response is closed before the
  // first streamController.enqueue() fires. Result: client gets shell-only HTML
  // with no closing tags, React Router suspends on data forever, page sits on
  // the route's fallback. Awaiting allReady (what bots used to get) buffers the
  // full response server-side and ships a complete document.
  await body.allReady;

  responseHeaders.set("Content-Type", "text/html; charset=utf-8");

  return new Response(body, {
    headers: responseHeaders,
    status: responseStatusCode,
  });
}
