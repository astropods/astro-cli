import { useEffect, useRef } from "react";
import type { DatasetJudgmentVerdict } from "@/lib/api";

const REVIEW_QUEUE_VERDICT_SHORTCUTS: Record<
  string,
  DatasetJudgmentVerdict
> = {
  g: "good",
  b: "bad",
  s: "unknown",
};

/** Binds a window keydown listener once per mount while always invoking the
 *  latest handler, so callers can pass inline closures without re-subscribing
 *  on every render. */
function useWindowKeyDown(handler: (event: KeyboardEvent) => void) {
  const handlerRef = useRef(handler);
  handlerRef.current = handler;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => handlerRef.current(event);
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
}

export function useReviewQueueShortcuts({
  disabled,
  onSelect,
  onAgree,
}: {
  disabled: boolean;
  onSelect: (verdict: DatasetJudgmentVerdict) => void;
  onAgree?: () => void;
}) {
  useWindowKeyDown((event) => {
    if (disabled) {
      return;
    }

    const agrees = event.key === "Enter";
    const verdict = REVIEW_QUEUE_VERDICT_SHORTCUTS[event.key.toLowerCase()];
    const run = agrees
      ? onAgree
      : verdict
        ? () => onSelect(verdict)
        : undefined;
    if (
      !run ||
      // Enter natively activates a focused button or link, so it yields to one;
      // the letter shortcuts collide with nothing and always fire.
      shouldIgnoreReviewQueueShortcut(event, {
        ignoreInteractiveControls: agrees,
      })
    ) {
      return;
    }

    event.preventDefault();
    run();
  });
}

export function useReviewQueueNavigationShortcuts({
  disabled,
  onPrevious,
  onNext,
}: {
  disabled: boolean;
  onPrevious: () => void;
  onNext: () => void;
}) {
  useWindowKeyDown((event) => {
    const goesUp = event.key === "ArrowUp";
    if (disabled || (!goesUp && event.key !== "ArrowDown")) {
      return;
    }

    // Widgets that own arrow keys prevent default; otherwise, hand them to the queue.
    if (shouldIgnoreReviewQueueShortcut(event)) {
      return;
    }

    event.preventDefault();
    if (goesUp) {
      onPrevious();
    } else {
      onNext();
    }
  });
}

export function shouldIgnoreReviewQueueShortcut(
  event: KeyboardEvent,
  { ignoreInteractiveControls = false } = {},
) {
  return (
    event.defaultPrevented ||
    event.repeat ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    isEditableShortcutTarget(event.target) ||
    (ignoreInteractiveControls && isInteractiveShortcutTarget(event.target))
  );
}

function isEditableShortcutTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }

  return (
    target.isContentEditable ||
    Boolean(
      target.closest(
        "input, textarea, select, [contenteditable='true'], [contenteditable='plaintext-only']",
      ),
    )
  );
}

function isInteractiveShortcutTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  return target.closest("button, a") !== null;
}
