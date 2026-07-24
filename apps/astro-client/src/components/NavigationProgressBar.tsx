import { useNavigation, useRevalidator } from "react-router";
import { IndeterminateProgressBar } from "./IndeterminateProgressBar";

// Query activity is intentionally excluded because transitional resources poll.
export function NavigationProgressBar() {
  const navigation = useNavigation();
  const revalidator = useRevalidator();
  const active = navigation.state !== "idle" || revalidator.state !== "idle";
  return <IndeterminateProgressBar active={active} />;
}
