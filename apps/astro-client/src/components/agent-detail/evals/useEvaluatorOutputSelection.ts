import { useCallback, useMemo, useState } from "react";
import type { EvaluatorOutputValue } from "@/lib/api";

function valuesFromOutputs(outputs?: Iterable<EvaluatorOutputValue>) {
  const values = new Map<string, unknown>();
  for (const output of outputs ?? []) {
    if (output.value !== null && output.value !== undefined) {
      values.set(output.key, output.value);
    }
  }
  return values;
}

export function useEvaluatorOutputSelection(
  evaluators: { key: string }[],
  initialOutputs?: Iterable<EvaluatorOutputValue>,
) {
  const initialValues = useMemo(
    () => valuesFromOutputs(initialOutputs),
    [initialOutputs],
  );
  const [overrides, setOverrides] = useState<Map<string, unknown>>(new Map());

  const setValue = useCallback((key: string, value: unknown) => {
    setOverrides((previous) => new Map(previous).set(key, value));
  }, []);

  const values = useMemo(() => {
    const merged = new Map(initialValues);
    for (const [key, value] of overrides) {
      if (value === undefined) {
        merged.delete(key);
      } else {
        merged.set(key, value);
      }
    }
    return merged;
  }, [initialValues, overrides]);

  const outputs = useMemo(
    () =>
      evaluators.flatMap(({ key }) => {
        const value = values.get(key);
        return value === undefined ? [] : [{ key, value }];
      }),
    [evaluators, values],
  );

  const editedKeys = useMemo(
    () =>
      new Set(
        evaluators
          .map(({ key }) => key)
          .filter(
            (key) =>
              overrides.has(key) && overrides.get(key) !== initialValues.get(key),
          ),
      ),
    [evaluators, initialValues, overrides],
  );

  return { values, setValue, outputs, editedKeys };
}
