import { forwardRef } from "react";
import type { LucideProps } from "lucide-react";

/** Lucide v1 removed brand icons; drop-in for the old `Github` export. */
export const Github = forwardRef<SVGSVGElement, LucideProps>(
  ({ className, size = 24, strokeWidth = 2, ...props }, ref) => (
    <svg
      ref={ref}
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden
      {...props}
    >
      <path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-.1.58-.18.85-.26A10.15 10.15 0 0 0 22 3.95a9.5 9.5 0 0 0-6.5-3.3 9.65 9.65 0 0 0-3.4 0A9.5 9.5 0 0 0 3.52 3.95 10.15 10.15 0 0 0 1.85 9.06c-.28 1-.28 2.35.03 3.45A6.08 6.08 0 0 0 3 15.22v4" />
    </svg>
  ),
);
Github.displayName = "Github";
