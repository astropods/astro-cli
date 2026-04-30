# fix(cli): ast push hits wrong registry after domain migration

## Summary

`ast push` and `ast blueprint push` were failing with a DNS error (`no such host`) after upgrading to CLI 0.12.0. The root cause was that the push command always derived the registry URL from the API server URL, ignoring the `DefaultRegistryURL` build-time flag introduced to allow the registry and API server to live on different domains. After the API server was migrated to `astropods.com`, the derived registry URL became `registry.astropods.com`, which does not exist — the production registry is still at `registry.astropods.ai`.

## Design

The fix adds a `pushRegistryURL()` helper that mirrors the existing `pushBaseURL()` pattern:

1. **Test mode** (`pushServerURLOverride` set): derive registry URL from the test server, preserving existing test behaviour.
2. **Production build** (`DefaultRegistryURL` non-empty): use the ldflag value directly (`https://registry.astropods.ai`).
3. **Local dev** (neither set): derive from `DefaultServerURL` as before.

`runPush` now calls `pushRegistryURL()` instead of `auth.RegistryURLFromServerURL(effectiveServerURL)` directly.

A `TestPushRegistryURL` table-driven test covers all three cases.

## Migration

No action required. Users on 0.12.0 who hit the DNS error should upgrade to the next release; the push command will use the correct registry automatically.
