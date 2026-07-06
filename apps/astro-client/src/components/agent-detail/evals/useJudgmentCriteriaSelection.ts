import { useCallback, useMemo, useState } from "react";
import type { EvalDatasetItemsVerdict } from "@/lib/api";
import { JUDGMENT_CRITERIA, toCriteria } from "./judgment-criteria";

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

  const selectedKeysInDisplayOrder = useMemo(
    () =>
      JUDGMENT_CRITERIA.filter((d) => selected.has(d.dimensionKey)).map(
        (d) => d.dimensionKey,
      ),
    [selected],
  );

  const selectedCriteriaForVerdict = useCallback(
    (verdict: EvalDatasetItemsVerdict) =>
      toCriteria(selectedKeysInDisplayOrder, verdict),
    [selectedKeysInDisplayOrder],
  );

  return { selected, dirty, toggle, selectedCriteriaForVerdict };
}
