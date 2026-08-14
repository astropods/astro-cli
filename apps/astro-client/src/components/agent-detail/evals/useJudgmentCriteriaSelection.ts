import { useCallback, useMemo, useState } from "react";
import type { JudgmentCriterion } from "@/lib/api";
import { JUDGMENT_CRITERIA, type CriterionValue } from "./judgment-criteria";

function selectionFromCriteria(criteria?: Iterable<JudgmentCriterion>) {
  const selection = new Map<string, CriterionValue>();
  for (const criterion of criteria ?? []) {
    if (criterion.value > 0) selection.set(criterion.dimension_key, 1);
    if (criterion.value < 0) selection.set(criterion.dimension_key, -1);
  }
  return selection;
}

export function useJudgmentCriteriaSelection(
  initialCriteria?: Iterable<JudgmentCriterion>,
) {
  const initialSelection = useMemo(
    () => selectionFromCriteria(initialCriteria),
    [initialCriteria],
  );
  const [selected, setSelected] =
    useState<Map<string, CriterionValue>>(initialSelection);

  const dirty =
    selected.size !== initialSelection.size ||
    Array.from(initialSelection).some(
      ([key, value]) => selected.get(key) !== value,
    );

  const toggle = useCallback((dimensionKey: string, value: CriterionValue) => {
    setSelected((prev) => {
      const next = new Map(prev);
      if (next.get(dimensionKey) === value) {
        next.delete(dimensionKey);
      } else {
        next.set(dimensionKey, value);
      }
      return next;
    });
  }, []);

  // Wire order follows the dimension display order, not click order.
  const criteria = useMemo(
    () =>
      JUDGMENT_CRITERIA.flatMap(({ dimensionKey }) => {
        const value = selected.get(dimensionKey);
        return value === undefined
          ? []
          : [{ dimension_key: dimensionKey, value }];
      }),
    [selected],
  );

  return { selected, dirty, toggle, criteria };
}
