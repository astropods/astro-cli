import { useNavigation, useRevalidator } from "react-router";
import { IndeterminateProgressBar } from "./IndeterminateProgressBar";

// Thin progress bar pinned to the top of the viewport that surfaces React
// Router navigation / revalidation activity. Tied to nav + revalidator only —
// NOT useIsFetching. Account-scoped queries with `placeholderData:
// keepPreviousData` keep old data on screen across key flips, so the loader
// signal is the right proxy for "the new page is ready." Watching every query
// causes the bar to get stuck whenever polling is active (deployment status,
// knowledge transitional states all poll every 3s).
export function NavigationProgressBar() {
  const navigation = useNavigation();
  const revalidator = useRevalidator();
  const active = navigation.state !== "idle" || revalidator.state !== "idle";
  return <IndeterminateProgressBar active={active} />;
}
