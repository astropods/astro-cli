# Keep spend limits saving when a response carries no body

## Summary

Saving spend limits crashes when the provider answers a successful save with an
empty body. The save itself lands, but the browser throws while reading the
response, so the rest of the handler never runs: the "still stopped" banner
keeps its previous value, no toast appears, and the form looks stuck even though
the limit changed.

## Design

`SpendControls` saves every edited row through `Promise.allSettled`, then scans
the results for a provider-side signal that lifting the spend stop failed. That
scan read the field off each fulfilled result directly, which assumes every
successful save resolves to an object. A save that resolves to nothing is a
normal outcome, so the scan now tolerates it and treats a bodyless response as
"the stop was not reported as failed".

The read is the only place that assumed a body. Rejected saves already flow
through the existing error path, and a partial save still clears every edit so
the form goes back to reading through the cache.

## Migration

None.
