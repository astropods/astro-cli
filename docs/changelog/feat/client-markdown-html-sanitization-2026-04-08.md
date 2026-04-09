# Markdown HTML Rendering with GitHub-Compatible Sanitization

## Summary

Agent READMEs can now use raw HTML tags in markdown (e.g. `<details>`, `<kbd>`, `<mark>`, `<sub>`, `<figure>`) matching GitHub's rendering behavior. Previously, `react-markdown` silently stripped all HTML. A sanitization layer ensures only safe tags and attributes are rendered.

## Design

**Parsing pipeline** -- Two rehype plugins are added to the `StyledMarkdown` component in sequence:

1. `rehype-raw` parses raw HTML embedded in markdown into the AST (instead of discarding it).
2. `rehype-sanitize` walks the AST and removes anything not on an explicit allowlist.

The order matters: HTML is parsed into structured nodes first, then sanitized. This prevents bypasses where malformed HTML might survive string-level filtering but gets caught once parsed into a proper tree.

**Sanitization schema** -- The schema extends `rehype-sanitize`'s `defaultSchema` (itself modeled on GitHub's `html-pipeline` sanitization filter) with 11 additional tags GitHub allows: `abbr`, `bdo`, `caption`, `cite`, `dfn`, `figure`, `figcaption`, `mark`, `small`, `time`, `wbr`. It also adds `loading` on `img` and `dateTime` on `time`. The `strip` list fully removes `script` and `style` tags including their children.

**Security model** -- The sanitizer uses an allowlist architecture, meaning nothing passes through unless explicitly permitted:

- **Allowed tags**: ~60 tags matching GitHub's set (headings, text formatting, lists, tables, code, media, semantic elements, `<details>`/`<summary>`).
- **Allowed attributes**: A per-tag allowlist derived from GitHub's. Global attributes like `align`, `height`, `width`, `title`, `id` are permitted. Element-specific attributes (e.g. `href` on `<a>`, `src` on `<img>`, `cite` on `<blockquote>`) are scoped to their respective tags.
- **Allowed protocols**: Only `http`, `https`, `mailto`, `xmpp`, and `irc`/`ircs` on `href`; only `http`/`https` on `src`, `longdesc`, and `cite`. This blocks `javascript:`, `vbscript:`, `data:`, and `file:` URIs.
- **Stripped entirely**: `<script>` and `<style>` tags are removed along with all their children (not just the tag wrapper).
- **Stripped attributes**: Event handlers (`onclick`, `onerror`, `onload`, etc.), `style`, `class` (except specific GFM patterns like `task-list-item`), and any attribute not on the allowlist.
- **Clobber prevention**: User-supplied `id` and `name` attributes are prefixed with `user-content-` to prevent collisions with page structure.

This is the same defense-in-depth approach GitHub uses: the allowlist is the primary control, protocol filtering handles URI-based vectors, and React's `createElement` rendering (vs. `innerHTML`) provides an additional layer since the sanitized AST is never injected as a raw HTML string.

**Test coverage** -- 66 tests covering: script injection (direct, mixed-case, null-byte), event handler stripping, `javascript:`/`vbscript:`/`data:`/`file:` URI filtering, entity-encoded and whitespace-padded protocol evasion, dangerous tags (`iframe`, `object`, `embed`, `form`, `style`, `link`, `meta`, `base`), SVG/MathML vectors, mutation XSS patterns (`noscript`, `textarea`, `title`, CDATA), unquoted/backtick attributes, markdown-native `javascript:` links, fullwidth Unicode evasion, UTF-7 encoding, and positive tests verifying all allowed tags render correctly.

## Migration

No migration required. Existing READMEs that only use standard markdown are unaffected. READMEs that previously included HTML tags (which were silently dropped) will now render those tags if they are on the allowlist.
