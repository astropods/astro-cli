# Messaging sidecar log level override in `ast project start`

## Summary
The messaging sidecar's log level was hardcoded to `info` in the compose builder, hiding the noisy adapter detail (Slack auth, rate limits, event routing) that's most useful while iterating locally.

## Design
Two changes:
1. The compose builder now defaults the messaging sidecar's `LOG_LEVEL` to `debug` in local dev, since this code path only runs under `ast project start`.
2. Adds an optional `log_level` field on `dev.interfaces.messaging` in `astropods.yml` so users can opt back to `info`/`warn`/`error`. Read via a new `Dev.MessagingLogLevel()` accessor; empty falls through to the `debug` default.

```yaml
dev:
  interfaces:
    messaging:
      adapters: [slack]
      log_level: info
```

## Migration
None required. Existing specs get more verbose messaging logs in dev; set `log_level: info` to restore the prior behavior.
