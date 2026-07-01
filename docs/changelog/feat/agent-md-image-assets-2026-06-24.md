## Summary

AGENT.md files commonly reference local images (`![diagram](./docs/arch.png)`), the same way a GitHub README does. Until now we stored the raw markdown verbatim, so those relative links pointed at nothing once the agent card was rendered in the web app — the images simply broke. This change vacuums the referenced images into the shared assets bucket at blueprint-build time and rewrites the stored AGENT.md to link to their CDN URLs, on every path that produces a new blueprint version (CLI push and GitHub builds).

## Design

The work splits into a shared scanner, a storage layer, and per-path image acquisition that converges on a single rewrite.

**Shared scan/rewrite (`packages/astro-spec`).** `ExtractMarkdownImages` finds local image references in both markdown (`![]()`) and HTML (`<img src>`) form, skipping remote (`http(s)://`, `//`), `data:`, root-absolute (`/foo`), and parent-escaping (`../`) references. `RewriteMarkdownImages` replaces those references with mapped URLs. Both are pure and used by CLI and server alike, so there is exactly one definition of "what counts as a local image."

**Storage (`internal/readmeassets`).** A thin store over the existing avatar storage backend (same bucket, same CDN — no new infrastructure). Images are stored as-is (no resize) under content-addressed keys `readme-assets/{account}/{name}/{sha256}{ext}`, which makes re-pushes idempotent and the objects safe to cache. SVG is detected and served as `image/svg+xml`; other types via content sniffing.

**Acquisition differs by path; the rewrite is shared.** The two build paths have asymmetric access to the source files:

- *CLI push* — only the CLI has the local repo. A new pipeline step scans AGENT.md, reads each referenced image (sandboxed to the spec's working directory), and uploads them to a new `POST /agents/:account/:name/readme-assets` endpoint as `multipart/form-data`. The repo-relative path travels as each part's **field name** (multipart readers reduce the filename to a basename, but preserve the field name verbatim). The server stores each image and returns a path→URL map, which the CLI forwards in the register request.
- *GitHub build* — the server fetches each referenced image itself from the repo at the build commit via the GitHub contents API, and stores it.

Both paths funnel the resulting path→URL map through `RewriteMarkdownImages` **before** the agent card is parsed, so the stored readme and the parsed card body both link to the CDN.

**Failure is non-fatal.** A missing, oversized (>10 MB), or unsupported image is left as its original reference and logged — it never fails a push or a build. Per-AGENT.md image count is capped (20).

## Migration

No action required, and no infrastructure changes — the existing assets bucket, CDN, and server IAM permissions are reused. New pushes and GitHub builds vacuum images automatically. Existing blueprints are unchanged; their AGENT.md images are picked up the next time a new version is built. Older CLIs that don't send images continue to register normally (the new request field is optional).
