# Debounce list search at the input, not just at the query

## Summary

Typing or deleting a character in the Agents page search box blocked the main thread. An agent card is one of the most expensive components in the client (a 150-star SVG starfield plus two d3 spline sparklines), and a full page of them was reconciled on every keypress.

The list pages already debounced search. The debounce was just pointed at the wrong thing: it gated the term sent to the server, while the half-typed text still lived in page state. So every keystroke re-rendered the page and the result grid under it, to redraw a result set that by construction could not have changed yet: the query behind it hadn't fired.

The fix moves the debounce to where the keystrokes are. The search box owns its in-flight text and reports only the settled term, so pages re-render when the term settles instead of on every keypress. `/agents`, `/blueprints`, and `/knowledge` all shared the pattern and all now share the fix.

## Design

`DebouncedFilterInput` wraps `FilterInput` with local text state and the existing `useDebouncedValue` hook. It reports upward `debounceMs` after the last keystroke, and tracks the last term it and its owner agreed on so that a settled term is never reported twice and an external reset (a "Clear filters" button) is adopted without echoing back as a fresh edit.

One reset cannot travel through `value`. "Clear filters" is reachable while the term is still empty, because another filter can be active and matching nothing, and dropping an already-empty term is a state write that changes nothing, which React does not surface as a prop change. Text typed inside the debounce window would then outlive the clear and apply itself moments later as a search the user had just cancelled. So the pages bump a `resetKey` alongside the clear, and the box reads that as "adopt `value` even if it looks unchanged".

`useUserResourceSearch` correspondingly holds the settled term rather than the live text, and derives list params from it directly:

```ts
const [search, setSearch] = useState("");
const q = search.trim();
const params: UserResourceListParams = q ? { q } : {};
```

No memoization is involved. Query keys are hashed structurally and the one other consumer already spread `params` into a fresh object per render, so the object's identity was never load-bearing.

This also removes a class of derived state. Both `/blueprints` and `/knowledge` carried a `hasTypedSearch` flag alongside their real filter flag, existing only to describe the window where the user had typed but the debounce hadn't fired. That window is gone, so the flag is too, and the "no results match your filters" state now flips in step with the results it describes rather than up to 300ms ahead of them.

**Alternative considered:** memoizing the card so the grid bails out of the re-render. That treats a state-placement problem as a memoization problem: it leaves the page re-rendering per keystroke and makes correct behavior depend on every prop staying referentially stable forever. Keeping the text local removes the render instead of absorbing it, and measures better: the grid isn't reconciled at all, and neither is the page.

Measured on a 24-card page (jsdom, one synchronous change event per keystroke):

| | card renders per keystroke | wall time per keystroke |
|---|---|---|
| Before | 24 | 59.5 ms typing / 47.6 ms deleting |
| After | 0 | 0.5 ms typing / 0.2 ms deleting |

The debounce contract is pinned on the input itself (reports once when typing stops, reports an emptied box, silent on mount, adopts an external reset without echoing). A separate test pins the payoff at the grid: keystrokes are asserted synchronously, in the same tick they are dispatched, so no debounce can have elapsed and any card render observed is one the keystroke caused. Both fail if the in-flight text is lifted back into page state.

## Migration

No migration is required. Debounce timing is unchanged at 300 ms, so the term reaching the server and the number of requests per search are the same as before.
