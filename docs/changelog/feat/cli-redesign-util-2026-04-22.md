## Summary

Establishes shared utilities and authoring conventions for the CLI noun-verb redesign. Provides a common HTTP helper and URL builder that all command handlers must use, and documents the conventions in a CLAUDE.md so they are enforced consistently going forward.

## Design

`api.go` introduces two package-level helpers:

- `apiCall(ctx, method, url, body, token, verbose, &dest)` — handles JSON marshalling, Bearer auth, and response decoding in one call. Returns `"server returned status N"` for non-2xx responses so callers can map specific codes to user-friendly messages with `strings.Contains`.
- `apiPath(serverURL, account, operation, parts...)` — builds standard `/api/v1/{operation}/{account}/...` URLs, eliminating ad-hoc string construction across handlers.

`CLAUDE.md` codifies three authoring rules for all command handlers:
- Auth: always use `getCurrentAccountToken(ctx)` — never call auth internals directly.
- HTTP: always use `apiCall` — never create `http.Client` or manage response bodies manually.
- URLs: always use `apiPath` for standard paths; `strings.TrimSuffix + fmt.Sprintf` only for sub-resource suffixes.

## Migration

Nothing required for users. Internal convention only.
