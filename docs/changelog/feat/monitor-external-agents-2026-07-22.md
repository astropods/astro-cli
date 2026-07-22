# Monitor external agents: docs and opt-in content collection

## Summary

Astro can ingest usage telemetry from AI coding tools running outside the platform (developer machines, CI, other environments) through account-scoped ingest keys. That flow was undocumented publicly, and the generated managed-settings block enabled usage metadata only — there was no supported way to opt into collecting prompt/response text or tool inputs. This change adds public documentation for the flow and per-key toggles that turn content collection on at the source.

## Design

**Docs.** A new "Monitor external agents" section (an overview plus a first integration page) frames the capability around Astro's official integrations rather than arbitrary OpenTelemetry sources: create an ingest key, add a settings block to the tool's configuration, and its telemetry appears in the dashboard alongside deployed agents. Metadata-only by default; content collection is documented explicitly with its privacy implications.

**Content-collection toggles.** The ingest-key creation dialog gains two switches — "Collect prompts and responses" and "Store tool calls" — both off by default. They shape the copyable managed-settings block rather than acting as a live control: the effective setting is the forced environment in the customer's own admin console, which Astro cannot read or push. Enabling a toggle appends the logs-signal settings:

    OTEL_LOGS_EXPORTER    = otlp
    OTEL_LOG_USER_PROMPTS = 1     # prompt + response text
    OTEL_LOG_TOOL_DETAILS = 1     # tool inputs

The logs-exporter line is emitted once when either toggle is on. The ingest side already handles these records — the receiver transforms content-bearing log records into spans nested in the interaction's existing trace, so prompt/response and tool-input content surface as observations. The toggles complete the source half of that path.

## Migration

None. The documentation is additive and the toggles default off, so existing keys and settings blocks are unchanged. Admins who want content collection re-copy the block with the toggle enabled.
