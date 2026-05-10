import { useId } from "react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";

interface EarlyAdopterBadgeProps {
  accountNumber?: number;
}

/** Zigzag badge shown for early adopters (account_number ≤ 1000). */
export function EarlyAdopterBadge({ accountNumber }: EarlyAdopterBadgeProps) {
  const id = useId();
  // useId includes colons which are invalid in SVG ids — strip them
  const gradId = `ea-grad-${id.replace(/:/g, "")}`;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
    <span className="relative inline-flex items-center justify-center cursor-default">
      <svg
        className="absolute inset-0 h-full w-full"
        viewBox="0 0 100 26"
        preserveAspectRatio="none"
        aria-hidden="true"
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <linearGradient id={gradId} x1="0" y1="0" x2="100" y2="26" gradientUnits="userSpaceOnUse">
            <stop stopColor="#6D5BD0" />
            <stop offset="1" stopColor="#A1D1A8" />
          </linearGradient>
        </defs>
        <path
          d="M2.5 0 L97.5 0 L100 3.5 L97.5 8.5 L100 13 L97.5 18 L100 22.5 L97.5 26 L2.5 26 L0 22.5 L2.5 18 L0 13 L2.5 8.5 L0 3.5 Z"
          fill="var(--background)"
        />
        <path
          d="M2.5 0 L97.5 0 L100 3.5 L97.5 8.5 L100 13 L97.5 18 L100 22.5 L97.5 26 L2.5 26 L0 22.5 L2.5 18 L0 13 L2.5 8.5 L0 3.5 Z"
          fill={`url(#${gradId})`}
          fillOpacity="0.13"
        />
        <path
          d="M2.5 0 L97.5 0 L100 3.5 L97.5 8.5 L100 13 L97.5 18 L100 22.5 L97.5 26 L2.5 26 L0 22.5 L2.5 18 L0 13 L2.5 8.5 L0 3.5 Z"
          stroke={`url(#${gradId})`}
          strokeWidth="1"
          fill="none"
          strokeOpacity="0.5"
          vectorEffect="non-scaling-stroke"
        />
      </svg>
      {/* Brand indigo — intentional raw color, not a semantic token */}
      <span className="relative z-10 px-5 py-1.5 text-center text-label font-mono flex items-center gap-1.5" style={{ color: "#6D5BD0" }}>
        Early adopter
        {accountNumber != null && (
          <span>#{accountNumber}</span>
        )}
      </span>
    </span>
        </TooltipTrigger>
        <TooltipContent>
          One of the first 1,000 accounts on Astro
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
