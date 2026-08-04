import type { ReactNode } from "react";

interface ListResultsTransitionProps {
  transitionKey: string;
  children: ReactNode;
}

/** Replays a compositor-only entrance when a resolved list scope changes. */
export function ListResultsTransition({
  transitionKey,
  children,
}: ListResultsTransitionProps) {
  return (
    <div
      key={transitionKey}
      className="animate-in fade-in-0 slide-in-from-bottom-1 duration-150 ease-out motion-reduce:animate-none motion-reduce:transform-none"
    >
      {children}
    </div>
  );
}
