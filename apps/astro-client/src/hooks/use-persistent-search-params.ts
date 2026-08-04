import { useCallback, useEffect, useRef } from "react";
import { createSearchParams, useSearchParams } from "react-router";
import {
  getPersistentStorageSnapshot,
  setPersistentStorageSnapshot,
  subscribePersistentStorage,
} from "@/lib/persistent-storage";

function mergeParams(
  base: URLSearchParams | string,
  source: URLSearchParams,
  paramNames: readonly string[],
): URLSearchParams {
  const merged = new URLSearchParams(base);
  for (const name of paramNames) {
    merged.delete(name);
    for (const value of source.getAll(name)) {
      merged.append(name, value);
    }
  }
  return merged;
}

function writeFilters(
  storageKey: string,
  searchParams: URLSearchParams,
  paramNames: readonly string[],
) {
  const stored = getPersistentStorageSnapshot(storageKey) ?? "";
  const merged = mergeParams(stored, searchParams, paramNames);
  setPersistentStorageSnapshot(storageKey, merged.toString());
}

export function usePersistentSearchParams(
  storageScope: string,
  paramNames: readonly string[],
  options?: { atomic?: boolean },
): ReturnType<typeof useSearchParams> {
  const storageKey = `astro:page-filters:${storageScope}`;
  const [searchParams, setSearchParams] = useSearchParams();
  const initialized = useRef(false);

  useEffect(() => {
    if (initialized.current) return;
    initialized.current = true;
    const explicitParamNames = paramNames.filter((name) =>
      searchParams.has(name),
    );
    if (explicitParamNames.length > 0) {
      writeFilters(
        storageKey,
        searchParams,
        options?.atomic ? paramNames : explicitParamNames,
      );
    }

    const omittedParamNames = options?.atomic && explicitParamNames.length > 0
      ? []
      : paramNames.filter((name) => !searchParams.has(name));
    if (omittedParamNames.length === 0) return;
    const storedValue = getPersistentStorageSnapshot(storageKey);
    if (!storedValue) return;
    // A direct SSR load cannot read local storage, so restoring a non-default
    // scope here intentionally causes one client refetch after hydration.
    const stored = new URLSearchParams(storedValue);
    setSearchParams(
      (current) => mergeParams(current, stored, omittedParamNames),
      { replace: true },
    );
  }, [options?.atomic, paramNames, searchParams, setSearchParams, storageKey]);

  useEffect(
    () =>
      subscribePersistentStorage(storageKey, () => {
        // Local writes already update the URL. Ignore non-clear notifications
        // to avoid a storage/URL feedback loop; auth-scoped clears must sync.
        if (getPersistentStorageSnapshot(storageKey) !== null) return;
        setSearchParams(
          (current) => mergeParams(current, new URLSearchParams(), paramNames),
          { replace: true },
        );
      }),
    [paramNames, setSearchParams, storageKey],
  );

  const setPersistentSearchParams = useCallback<typeof setSearchParams>(
    (nextInit, navigateOptions) => {
      const committed = { current: null as URLSearchParams | null };
      setSearchParams((previous) => {
        const current = new URLSearchParams(previous);
        const resolved =
          typeof nextInit === "function" ? nextInit(current) : nextInit;
        const next = createSearchParams(resolved);
        committed.current = next;
        return next;
      }, navigateOptions);
      if (committed.current) {
        writeFilters(storageKey, committed.current, paramNames);
      }
    },
    [paramNames, setSearchParams, storageKey],
  );

  return [searchParams, setPersistentSearchParams];
}
