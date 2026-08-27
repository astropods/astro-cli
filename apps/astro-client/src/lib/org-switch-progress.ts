// Synchronous org-switch signal for NavigationProgressBar and the scope
// switcher. Lives outside ActiveAccountProvider state so flipping it does not
// re-render heavy outlets.
let target: string | null = null;
const listeners = new Set<() => void>();

export function setOrgSwitchTarget(next: string | null) {
  if (target === next) return;
  target = next;
  for (const listener of listeners) listener();
}

export function subscribeOrgSwitchProgress(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getOrgSwitchTarget() {
  return target;
}

export function getOrgSwitchProgress() {
  return target !== null;
}
