# Add refresh button to log viewer

## Summary

The log viewer had no way to re-fetch logs without changing the time range selector. Users had to switch to a different interval and back to trigger a reload. This adds an explicit refresh button that re-fetches the current time interval on demand.

## Design

A refresh button (`RefreshCw` icon) is added to the `LogViewer` toolbar between the live tail toggle and the copy button. It accepts two new optional props:

- `onRefresh?: () => void` — called when the button is clicked; button is hidden if absent
- `isRefetching?: boolean` — when true, disables the button and spins the icon

The button is also disabled when `isTailing` is true, since live tail already streams real-time data and a manual refetch would conflict.

In `LogsTab`, `refetch` is wired from `useDeploymentLogs`. To avoid spinning the button on initial load or time range changes, a local `isManualRefetching` flag is set on click and cleared when `isFetching` goes false:

```tsx
const [isManualRefetching, setIsManualRefetching] = useState(false);
useEffect(() => {
  if (!histFetching) setIsManualRefetching(false);
}, [histFetching]);

onRefresh={() => { setIsManualRefetching(true); void refetch(); }}
isRefetching={isManualRefetching}
```

The `queryFn` in `useDeploymentLogs` computes `since` as `new Date(Date.now() - ms)` at call time, so every refetch fetches the correct window relative to now.

Storybook's global decorator was updated to include `MemoryRouter` and `AuthContext.Provider` (needed by `useLogTimezone` via `useAuth`), fixing errors that affected all `LogViewer` stories.

## Migration

No changes required.
