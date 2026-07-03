import { useId } from "react";
import { cn } from "@/lib/utils";

/** Brand mark (rocket) from astro-logo.svg with a gentle float animation. */
export function AstroLogoLoader({
  className,
  size = 64,
}: {
  className?: string;
  /** Width in px; height follows the logo aspect ratio. */
  size?: number;
}) {
  const uid = useId().replace(/:/g, "");
  const gradA = `astro-logo-loader-a-${uid}`;
  const gradB = `astro-logo-loader-b-${uid}`;
  const height = Math.round(size * (202 / 230));

  return (
    <div
      className={cn("relative flex items-center justify-center", className)}
      style={{ width: size, height }}
      role="status"
      aria-label="Loading"
    >
      <span
        className="pointer-events-none absolute -inset-1.5 rounded-full bg-primary/7 animate-ping"
        style={{ animationDuration: "2.4s" }}
        aria-hidden
      />
      <svg
        viewBox="0 0 230 202"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="astro-logo-loader relative h-full w-full"
        aria-hidden
      >
        <defs>
          <linearGradient
            id={gradA}
            x1="140.287"
            y1="222.172"
            x2="97.5135"
            y2="116.921"
            gradientUnits="userSpaceOnUse"
          >
            <stop stopColor="#6D5BD0" />
            <stop offset="1" stopColor="#A1D1A8" />
          </linearGradient>
          <linearGradient
            id={gradB}
            x1="134.829"
            y1="111.588"
            x2="55.2685"
            y2="108.108"
            gradientUnits="userSpaceOnUse"
          >
            <stop stopColor="#6D5BD0" />
            <stop offset="1" stopColor="#A1D1A8" />
          </linearGradient>
        </defs>
        <path
          d="M20.6273 3.47349C21.2094 1.42565 23.1658 -0.0397221 25.2643 0.000820906L223.119 3.85811C224.45 3.88404 225.64 4.51777 226.347 5.57695C227.053 6.63617 227.195 8.00172 226.732 9.27834L158.08 198.833C157.349 200.85 155.295 202.18 153.216 201.98C151.137 201.78 149.638 200.111 149.663 198.027L150.787 105.442L150.623 105.639L148.286 68.0117C147.903 61.8542 144.21 56.6455 138.589 54.3319L108.251 41.844L108.305 41.826L22.9061 8.30737C21.0075 7.56215 20.0456 5.52167 20.6273 3.47349Z"
          fill={`url(#${gradA})`}
        />
        <path
          className="astro-logo-loader-body"
          d="M0.0123279 81.2806C-0.146831 79.1878 1.25023 77.1519 3.32752 76.4481L107.009 41.3323L138.589 54.3319C144.21 56.6455 147.902 61.853 148.285 68.0105L150.666 106.313L150.525 106.561L150.53 106.116L80.0526 191.386C78.6431 193.091 76.2853 193.66 74.4498 192.737C72.6145 191.813 71.8251 189.663 72.574 187.626L104.579 100.622C107.186 93.5348 102.503 86.4982 95.1014 86.3823L3.97497 84.9552C1.84164 84.9217 0.171803 83.3733 0.0123279 81.2806Z"
          fill={`url(#${gradB})`}
        />
      </svg>
    </div>
  );
}
