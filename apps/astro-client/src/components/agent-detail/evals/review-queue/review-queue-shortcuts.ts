import { useEffect, useRef } from "react";

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

function shouldIgnoreReviewQueueShortcut(
  event: KeyboardEvent,
) {
  return (
    event.defaultPrevented ||
    event.repeat ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey ||
    isEditableShortcutTarget(event.target)
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
