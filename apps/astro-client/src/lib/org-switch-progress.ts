// Synchronous org-switch signal for NavigationProgressBar only. Lives outside
// ActiveAccountProvider state so flipping it does not re-render heavy outlets.
let switching = false;
const listeners = new Set<() => void>();

export function setOrgSwitchProgress(on: boolean) {
  if (switching === on) return;
  switching = on;
  for (const listener of listeners) listener();
}

export function subscribeOrgSwitchProgress(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getOrgSwitchProgress() {
  return switching;
}
