import { useMemo, useRef } from "react";

// ---------------------------------------------------------------------------
// Field categories — each tracked field declares whether a change is cosmetic
// (save without redeploy) or requires a full redeploy.
// ---------------------------------------------------------------------------

type ChangeCategory = "cosmetic" | "redeploy";

interface TrackedField<T> {
  category: ChangeCategory;
  /** Return true when current value differs from initial. */
  isChanged: (initial: T, current: T) => boolean;
}

// Comparators
const stringChanged = (a: string, b: string) => a !== b;
const stringArrayChanged = (a: string[], b: string[]) =>
  a.length !== b.length || a.some((v, i) => v !== b[i]);
const recordChanged = (a: Record<string, string>, b: Record<string, string>) => {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  if (aKeys.length !== bKeys.length) return true;
  return aKeys.some((k) => a[k] !== b[k]);
};

// ---------------------------------------------------------------------------
// Field registry — add new tracked fields here.
// ---------------------------------------------------------------------------

export interface TrackedFormState {
  deployName: string;
  variableValues: Record<string, string>;
  selectedAdapters: string[];
  adapterCredentials: Record<string, string>;
}

const FIELD_CONFIG: { [K in keyof TrackedFormState]: TrackedField<TrackedFormState[K]> } = {
  deployName:          { category: "cosmetic", isChanged: stringChanged },
  variableValues:      { category: "redeploy", isChanged: recordChanged },
  selectedAdapters:    { category: "redeploy", isChanged: stringArrayChanged },
  adapterCredentials:  { category: "redeploy", isChanged: recordChanged },
};

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export interface ChangeTrackingResult {
  /** True when any field differs from initial values. */
  isDirty: boolean;
  /** True when at least one redeploy-category field has changed. */
  requiresRedeploy: boolean;
  /** True when changes exist but all are cosmetic (no redeploy needed). */
  cosmeticOnly: boolean;
  /** Per-field dirty flags for fine-grained UI (e.g. highlighting changed sections). */
  dirtyFields: Record<keyof TrackedFormState, boolean>;
}

export function useChangeTracking(current: TrackedFormState): ChangeTrackingResult {
  // Capture the first non-undefined snapshot as the baseline.
  const initialRef = useRef<TrackedFormState | null>(null);
  if (initialRef.current === null) {
    initialRef.current = structuredClone(current);
  }
  const initial = initialRef.current;

  return useMemo(() => {
    const dirtyFields = {} as Record<keyof TrackedFormState, boolean>;
    let hasCosmeticChange = false;
    let hasRedeployChange = false;

    for (const key of Object.keys(FIELD_CONFIG) as (keyof TrackedFormState)[]) {
      const config = FIELD_CONFIG[key];
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const changed = config.isChanged(initial[key] as any, current[key] as any);
      dirtyFields[key] = changed;
      if (changed) {
        if (config.category === "cosmetic") hasCosmeticChange = true;
        else hasRedeployChange = true;
      }
    }

    const isDirty = hasCosmeticChange || hasRedeployChange;

    return {
      isDirty,
      requiresRedeploy: hasRedeployChange,
      cosmeticOnly: isDirty && !hasRedeployChange,
      dirtyFields,
    };
  }, [initial, current]);
}
