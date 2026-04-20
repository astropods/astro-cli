# Timezone support for log timestamps

## Summary

Log timestamps were always displayed in UTC, making it difficult for users in other timezones to correlate log times with their local clock. This change adds a user-configurable timezone preference that is applied server-side when fetching logs, so all timestamps — historical and live-streamed — are returned already converted to the user's chosen timezone.

## Design

### Preference storage

A timezone preference is stored in `localStorage` under `astro:log-timezone` (defaulting to `"UTC"`), following the same pattern as the existing `astro:theme` preference. The `useLogTimezone` hook in `src/lib/timezone.ts` reads and writes this value and syncs it across tabs via the `storage` event.

### Settings page

A "Preferences" section was added to `/settings/account` (`AccountSettings.tsx`) containing a searchable timezone selector. The selector (`src/components/ui/timezone-select.tsx`) is a Radix `Popover` with a virtualized list (using the existing `@tanstack/react-virtual` dependency) of all IANA timezone names, each labelled with its current GMT offset — e.g. `(GMT-07:00) America/Denver`. Selecting a value writes to localStorage and flashes a "Saved" indicator inline.

### Server-side conversion

The timezone is passed as a `?timezone=` query parameter on both the snapshot and streaming log endpoints. The server loads a `*time.Location` via `time.LoadLocation` (defaulting to `time.UTC` for missing or invalid values) and passes it through to `lokiLineToEntry`, which uses `ll.Timestamp.In(loc).Format(time.RFC3339Nano)` instead of the previous `ll.Timestamp.UTC()`. This means timestamps arrive at the client already in the correct timezone, carrying their UTC offset in the RFC3339 string (e.g. `2026-04-20T11:30:45.123-04:00`).

The same `?timezone=` parameter is supported on the knowledge store log endpoints (`handlers/knowledge.go`) since they share the same `streamLogs` and `lokiLineToEntry` functions.

### Frontend display

Because the server now handles conversion, `formatLogTimestamp` in `log-utils.ts` reverts to its original regex-based approach — it extracts the date and time parts from the RFC3339 string and ignores the offset. The timezone query key is included in TanStack Query's cache key so the historical log query refetches automatically when the timezone changes.

### Log viewer surface

The currently active timezone is shown as a teal link at the bottom of the time range dropdown in the log viewer toolbar. Clicking it navigates to `/settings/account` to change it.

### Query key

`deploymentKeys.logs` in `src/api/queries/keys.ts` now includes `timezone` so cached results for different timezones are stored independently.

## Migration

No action required. The timezone defaults to UTC, which preserves the existing behaviour for all users who do not configure a preference.
