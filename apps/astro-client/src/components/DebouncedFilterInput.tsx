import { useEffect, useRef, useState } from "react";
import { FilterInput, type FilterInputProps } from "@/components/FilterInput";
import { useDebouncedValue } from "@/hooks/use-debounced-value";

const DEFAULT_DEBOUNCE_MS = 300;

export interface DebouncedFilterInputProps
  extends Omit<FilterInputProps, "value" | "defaultValue" | "onChange"> {
  /** The term the list is currently filtered by. Changing it from outside
   *  (e.g. a "Clear filters" button) resets the text in the box. */
  value: string;
  /** Called with the settled text, `debounceMs` after the last keystroke. */
  onDebouncedChange: (value: string) => void;
  debounceMs?: number;
  /** Bump to discard in-flight text. A reset can leave `value` untouched (a
   *  filter set matching nothing without a search term in it), and an
   *  unchanged prop is not observable, so the reset is signalled separately. */
  resetKey?: number;
}

/**
 * Search box that keeps its in-flight text to itself and reports only the
 * settled term upward.
 *
 * List pages debounce the term they send to the server, but they still owned
 * the half-typed text, so every keypress re-rendered the page, including a
 * result grid that cannot have changed yet because the query behind it is
 * debounced. Holding the text here scopes a keystroke to this component and
 * leaves the page to re-render once, when the term actually settles.
 */
export function DebouncedFilterInput({
  value,
  onDebouncedChange,
  debounceMs = DEFAULT_DEBOUNCE_MS,
  resetKey,
  ...props
}: DebouncedFilterInputProps) {
  const [text, setText] = useState(value);
  const debouncedText = useDebouncedValue(text, debounceMs);
  // The term this box and its owner last agreed on. Guarding both directions
  // keeps a settled term from being reported twice, and keeps an external
  // reset from being echoed straight back.
  const committedRef = useRef(value);
  const resetKeyRef = useRef(resetKey);

  useEffect(() => {
    if (debouncedText === committedRef.current) return;
    committedRef.current = debouncedText;
    onDebouncedChange(debouncedText);
  }, [debouncedText, onDebouncedChange]);

  useEffect(() => {
    // A reset adopts `value` even when it matches the agreed term, since the
    // point is to drop text the owner has not heard about yet.
    const isReset = resetKey !== resetKeyRef.current;
    resetKeyRef.current = resetKey;
    if (!isReset && value === committedRef.current) return;
    committedRef.current = value;
    setText(value);
  }, [value, resetKey]);

  return (
    <FilterInput
      {...props}
      value={text}
      onChange={(event) => setText(event.target.value)}
    />
  );
}
