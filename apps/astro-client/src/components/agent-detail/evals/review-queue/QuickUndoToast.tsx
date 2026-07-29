import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { Button } from "@/components/ui/button";

const QUICK_UNDO_TIMEOUT_MS = 8000;

export interface QuickUndoToastProps {
  /** Header label, e.g. "Marked as neutral". */
  label: string;
  isUndoing: boolean;
  onUndo: () => void;
  /** Called when the toast auto-dismisses after the timeout elapses. */
  onDismiss: () => void;
}

/** Compact confirmation toast with an Undo that auto-dismisses after a timeout. */
export function QuickUndoToast({
  label,
  isUndoing,
  onUndo,
  onDismiss,
}: QuickUndoToastProps) {
  const onDismissRef = useRef(onDismiss);
  onDismissRef.current = onDismiss;

  useEffect(() => {
    const timer = window.setTimeout(
      () => onDismissRef.current(),
      QUICK_UNDO_TIMEOUT_MS,
    );
    return () => window.clearTimeout(timer);
  }, []);

  if (typeof document === "undefined") {
    return null;
  }

  return createPortal(
    <div
      aria-live="polite"
      className="fixed bottom-6 left-1/2 z-50 flex w-[calc(100vw-2rem)] max-w-md -translate-x-1/2 animate-in items-center justify-between gap-3 rounded-md border border-border bg-card py-2.5 pl-4 pr-3 text-body-sm text-foreground shadow-xl fade-in slide-in-from-bottom-2 duration-200 sm:w-auto sm:min-w-[300px]"
    >
      <span className="min-w-0 truncate">{label}</span>
      <Button
        type="button"
        variant="outline"
        size="xs"
        disabled={isUndoing}
        onClick={onUndo}
        className="h-7 flex-none px-3 font-semibold"
      >
        {isUndoing ? "Undoing..." : "Undo"}
      </Button>
    </div>,
    document.body,
  );
}
