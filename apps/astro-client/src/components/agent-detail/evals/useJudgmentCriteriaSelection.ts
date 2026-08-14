import { useCallback, useMemo, useState } from "react";

export function useJudgmentCriteriaSelection(initialKeys?: Iterable<string>) {
  const initialKeySet = useMemo(() => new Set(initialKeys ?? []), [initialKeys]);
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(initialKeys ?? []),
  );

  const dirty =
    selected.size !== initialKeySet.size ||
    Array.from(initialKeySet).some((key) => !selected.has(key));

  const toggle = useCallback((dimensionKey: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(dimensionKey)) {
        next.delete(dimensionKey);
      } else {
        next.add(dimensionKey);
      }
      return next;
    });
  }, []);

  return { selected, dirty, toggle };
}
