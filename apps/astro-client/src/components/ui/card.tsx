import * as React from "react";

import { cn } from "@/lib/utils";

/**
 * Base chrome for any lifted tile in the app: rounded, bordered, sits on
 * the `--card` token so it themes correctly across light and dark modes.
 *
 * Padding and any other layout (height, position, hover state, ...) are
 * passed through `className` — `<Card>` deliberately stays minimal so
 * authors don't reach for raw color utilities (`bg-white`, `bg-stone-*`,
 * `bg-surface`) when they want a card.
 */
function Card({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="card"
      className={cn(
        "rounded-[10px] border border-border bg-card text-card-foreground",
        className,
      )}
      {...props}
    />
  );
}

export { Card };
