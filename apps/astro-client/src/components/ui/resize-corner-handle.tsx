import type { SVGProps } from "react";
import { cn } from "@/lib/utils";

export function ResizeCornerHandle({
  className,
  ...props
}: SVGProps<SVGSVGElement>) {
  return (
    <svg
      aria-hidden
      data-resize-corner
      viewBox="0 0 8 8"
      {...props}
      className={cn("size-2 text-muted-foreground/80", className)}
    >
      <path
        d="M7 1L1 7"
        fill="none"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.25"
      />
    </svg>
  );
}
