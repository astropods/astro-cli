# astro-client: api.ts cleanup

## Summary

`apps/astro-client/src/lib/api.ts` had grown to ~2,300 lines with five near-duplicate fetch implementations, a silently-shadowed duplicate `ValidationError` interface, and types/methods scattered without a consistent domain grouping. This pass collapses the duplication and reorders the file so types and methods for each domain sit together. The module remains a single file with the same public surface — no callers need to change.

## Design

**One fetch path.** A new private `_fetch<T>(url, init, opts?)` helper owns the entire request lifecycle: credential cookies, header merging, JSON content-type default (skipped for FormData uploads), `ApiRequestError` mapping, and empty-body parsing. `request`, `authRequest`, and `uploadFormData` are now one-line wrappers that vary only by URL base and Content-Type handling:

```ts
private request<T>(endpoint: string, init: RequestInit = {}): Promise<T> {
  return this._fetch<T>(`${this.baseUrl}${endpoint}`, init);
}

private authRequest<T>(endpoint: string, init: RequestInit = {}): Promise<T> {
  return this._fetch<T>(`${this.authUrl}${endpoint}`, init);
}
```

The two endpoints that previously hand-rolled their own `fetch` calls (`getDeploymentLogs`, `getKnowledgeLogs`) now route through `request<LogEntry[]>(...)` so they share the same error mapping and fallback message shape as every other request.

**Linear top-to-bottom layout.** The file now reads in a single domain order: imports → helpers → core errors → all exported types (grouped by domain) → `ApiClient` class with methods in the same domain order → singleton. Each section is fronted by a banner header so a reader looking for, say, Knowledge can locate both the types and the methods by scrolling once.

Section order: Core errors, Avatars (cross-domain primitives), Auth & profile, Accounts, Blueprints (incl. hearts/archive), Deployments (spec + template + lifecycle + inspection + pod metrics + configmap/secret), Observability, Network, Knowledge, GitHub, Slack, Variables, Audit log, Usage/quota/feedback.

Avatars stay as their own section in both the types and methods regions because the FormData-upload pattern repeats across three resources (accounts, blueprints, deployments) — grouping them keeps that pattern visible. SSE URL builders (`getDeploymentLogsStreamUrl`, `getKnowledgeLogsStreamUrl`, `getKnowledgeEventsStreamUrl`) sit next to their request-mode siblings in their owning domains.

**Two small fixes during the pass:**
- The duplicate `ValidationError` interface (declared twice with identical shape) collapsed to one definition.
- `createBlueprint`, previously orphaned between deployment-avatar and knowledge methods, moved into the blueprints section next to `getBlueprint`.

## Migration

None. No exported types or method signatures changed. No call sites need updating. Typecheck and the full vitest suite (1,150 tests) pass unchanged.
