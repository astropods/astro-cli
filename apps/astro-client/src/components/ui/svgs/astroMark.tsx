import { useId, type SVGProps } from "react";

/** Astro rocket brand mark, sourced from astro-logo-dark.svg (the variant with a
 *  gap between the two shapes). Defaults to the brand gradient; pass `mono` to
 *  fill with currentColor (for small/tinted contexts, matching the monochrome
 *  logo treatment in the app header). */
const AstroMark = ({ mono, ...props }: SVGProps<SVGSVGElement> & { mono?: boolean }) => {
  const uid = useId().replace(/:/g, "");
  const idA = `astro-mark-a-${uid}`;
  const idB = `astro-mark-b-${uid}`;
  const fillA = mono ? "currentColor" : `url(#${idA})`;
  const fillB = mono ? "currentColor" : `url(#${idB})`;
  return (
    <svg {...props} viewBox="0 0 230 202" fill="none" xmlns="http://www.w3.org/2000/svg">
      {!mono && (
        <defs>
          <linearGradient id={idA} x1="140.287" y1="222.172" x2="97.5135" y2="116.921" gradientUnits="userSpaceOnUse">
            <stop stopColor="#6D5BD0" />
            <stop offset="1" stopColor="#A1D1A8" />
          </linearGradient>
          <linearGradient id={idB} x1="134.829" y1="111.588" x2="55.2685" y2="108.108" gradientUnits="userSpaceOnUse">
            <stop stopColor="#6D5BD0" />
            <stop offset="1" stopColor="#A1D1A8" />
          </linearGradient>
        </defs>
      )}
      <path
        d="M20.63 3.46994C21.21 1.42994 23.17 -0.0400576 25.26 -5.75925e-05L223.11 3.85994C224.44 3.88994 225.63 4.51994 226.34 5.57994C227.05 6.63994 227.19 7.99994 226.72 9.27994L158.07 198.83C157.34 200.85 155.28 202.18 153.21 201.98C151.13 201.78 149.63 200.11 149.66 198.03L150.78 105.45L150.62 105.65L148.28 68.0199C147.9 61.8599 144.2 56.6499 138.58 54.3399L108.24 41.8499L108.29 41.8299L22.91 8.30994C21.01 7.55994 20.05 5.51994 20.63 3.47994V3.46994Z"
        fill={fillA}
      />
      <path
        d="M142.76 108.01L142.64 106.13L140.3 68.5C140.11 65.4 138.33 62.87 135.54 61.72L105.2 49.23L103.21 48.41L97.9901 46.36C96.2301 45.67 94.2901 45.62 92.5001 46.23L3.33008 76.45C1.25008 77.15 -0.139919 79.19 0.0100814 81.28C0.170081 83.37 1.84008 84.92 3.97008 84.95L95.1001 86.38C102.5 86.5 107.18 93.53 104.58 100.62L72.5801 187.62C71.8301 189.66 72.6201 191.81 74.4601 192.73C76.3001 193.65 78.6501 193.08 80.0601 191.38L140.87 117.81C142.03 116.4 142.68 114.64 142.7 112.81L142.76 108.01Z"
        fill={fillB}
      />
    </svg>
  );
};

export { AstroMark };
