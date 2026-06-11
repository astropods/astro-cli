## Summary

Hard-refreshing `/insights` for the Postman organization rendered the page shell
(loading placeholders) and stayed there indefinitely — no console errors, no
network errors, no hydration warnings. Clicking from the navbar worked fine.
Other organizations were unaffected.

Root cause: the SSR response was being **truncated mid-stream** for browser
user agents on accounts with non-trivial loader data. The HTML body ended right
after React Router's `streamController` setup script — before the first
`streamController.enqueue(...)` call ever fired and before `</body></html>`.
The client received a shell with an unresolved `<!--$?-->` Suspense
placeholder, suspended forever waiting for data that would never arrive, and
never committed — so no useEffect ever ran and nothing in the page subtree
hydrated.

The trigger was a UA-conditional branch in `entry.server.tsx`:

- **Bot UAs** (curl, wget, Googlebot, plain `Mozilla/5.0`, etc.): the server
  did `await body.allReady` before responding, buffering the full SSR
  document. These got complete responses.
- **Real browser UAs** (Chrome, Firefox, Safari, Edge): the server returned
  the `ReadableStream` directly. Somewhere in the Bun.serve → ALB →
  CloudFront → Cloudflare pipeline, this streaming response was being
  truncated for any payload large enough to make the turbo-stream encoder
  yield to the event loop (every 6k items or 1ms, whichever comes first).

Bisected by curl, holding cookies / encoding constant and varying only the
UA: every UA `isbot` classified as `true` returned a complete 281KB document;
every browser UA returned 37KB truncated at the same byte offset. The org
specificity was just size — Postman's loader payload happened to cross the
threshold that triggers the encoder's first yield, smaller orgs slid by.

## Design

Dropped the UA-conditional and always buffer the SSR response server-side:

```tsx
const body = await renderToReadableStream(<ServerRouter ... />, { onError });
await body.allReady;
return new Response(body, { headers, status });
```

This matches what every account already saw via the `/insights.data`
navigation path (which doesn't stream) and what bot UAs were already getting,
so it aligns all paths on the same proven behavior.

Trade-off accepted: cold-refresh TTFB regresses on the largest payloads —
about 1–2 seconds for the slowest account, sub-200ms for typical accounts —
because the server now waits for the full encode before sending the first
byte. Progressive HTML was nice in principle, but it's only nice when it
actually works.

The `isbot` dependency was also dropped — it had no other usages.

This is a workaround, not a fix for the underlying streaming bug. The actual
defect is in the Bun.serve + proxy chain's handling of streamed
`ReadableStream` responses for browser UAs, and it deserves a separate
investigation. The diagnostic to start with: hit the Bun server directly
(bypassing ALB and CloudFront) with a browser UA and a large loader payload,
see if the truncation still reproduces. If yes → Bun. If no → proxy.

## Migration

None. The change is transparent — all accounts get the same complete SSR
document they always got on navigation, just now also on refresh.
