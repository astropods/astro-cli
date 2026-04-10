interface AgentMascotsIllustrationProps {
  size?: number;
  className?: string;
}

/**
 * Animated trio of agent mascots — gear (purple), star (teal), square (lavender).
 * Rendered as a single SVG so keyframes are declared once and the layout is
 * self-contained. Each mascot floats and winks with staggered delays.
 */
export function AgentMascotsIllustration({ size = 52, className }: AgentMascotsIllustrationProps) {
  const gap = Math.round(size * 0.23);
  const w = size * 3 + gap * 2;

  return (
    <svg width={w} height={size} viewBox={`0 0 ${w} ${size}`} overflow="visible" className={className}>
      <style>{`
        @keyframes mascot-float {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(-4px); }
        }
        @keyframes mascot-wink {
          0%, 85%, 100% { transform: scaleY(1); }
          90% { transform: scaleY(0.1); }
        }
      `}</style>

      {/* Gear — purple spiky, float delay 0s, wink delay 0s */}
      <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-float 3s ease-in-out 0s infinite" }}>
        <svg x={0} y={0} width={size} height={size} viewBox="0 0 60 60">
          <path d="M16.2395 1.62574C16.5771 0.2773 18.2888 -0.431743 19.481 0.283012C22.5018 2.09403 26.0366 3.13564 29.8149 3.13564C33.5927 3.13558 37.1269 2.0942 40.1473 0.283515C41.3395 -0.431174 43.0511 0.277822 43.3887 1.62617C44.2443 5.04279 46.0075 8.27889 48.6791 10.9505C51.3506 13.6219 54.5863 15.3842 58.0025 16.2394C59.3509 16.577 60.06 18.2886 59.3453 19.4809C57.5345 22.5017 56.4932 26.0366 56.4932 29.8148C56.4933 33.5927 57.5347 37.127 59.3455 40.1474C60.0601 41.3395 59.3512 43.0511 58.0029 43.3888C54.5865 44.2444 51.3507 46.0076 48.6791 48.6791C46.0076 51.3507 44.2443 54.5868 43.3888 58.0033C43.0511 59.3517 41.3395 60.0607 40.1473 59.3459C37.1269 57.535 33.5928 56.4932 29.8149 56.4932C26.0368 56.4932 22.5022 57.5346 19.4815 59.3454C18.2894 60.0601 16.5778 59.3511 16.2401 58.0028C15.3845 54.5864 13.6213 51.3506 10.9498 48.6791C8.2781 46.0075 5.04222 44.2447 1.62571 43.3894C0.277292 43.0518 -0.431746 41.3401 0.28302 40.1479C2.09397 37.1274 3.13565 33.5929 3.13568 29.8148C3.13568 26.0367 2.09436 22.5022 0.283592 19.4816C-0.431092 18.2894 0.2779 16.5778 1.62627 16.2401C5.04256 15.3847 8.27826 13.6219 10.9498 10.9505C13.6214 8.27883 15.3842 5.0425 16.2395 1.62574Z" fill="#7261D2" />
          <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-wink 4s ease-in-out 0s infinite" }}>
            <path d="M27.6953 32.7c0-3.60712-2.9241-6.53127-6.53122-6.53127s-6.53127 2.92415-6.53127 6.53127" stroke="white" strokeWidth="3.26563" fill="none" strokeLinecap="round" />
          </g>
          <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-wink 4s ease-in-out 0s infinite" }}>
            <path d="M44.8399 32.7c0-3.60712-2.9241-6.53127-6.5312-6.53127s-6.5313 2.92415-6.5313 6.53127" stroke="white" strokeWidth="3.26563" fill="none" strokeLinecap="round" />
          </g>
        </svg>
      </g>

      {/* Star — teal flower, float delay 0.5s, wink delay 1.5s */}
      <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-float 3s ease-in-out 0.5s infinite" }}>
        <svg x={size + gap} y={0} width={size} height={size} viewBox="0 0 60.6502 60.6502">
          <path d="M30.3251 0C35.0297 2.7513e-06 38.9008 3.56255 39.3909 8.13698C42.9682 5.35992 48.136 5.61185 51.4218 8.89758C54.7967 12.2727 54.9707 17.6333 51.9488 21.2163C56.7912 21.4348 60.6502 25.4287 60.6502 30.3251C60.6502 35.1146 56.9578 39.0386 52.2645 39.4124C55.3193 42.9954 55.1554 48.3818 51.7686 51.7686C48.3816 55.1554 42.9954 55.3188 39.4124 52.2637C39.0391 56.9575 35.1149 60.6502 30.3251 60.6502C25.5353 60.6502 21.6103 56.9575 21.237 52.2637C17.654 55.3181 12.2691 55.1551 8.88243 51.7686C5.4957 48.3819 5.3312 42.9954 8.38573 39.4124C3.6924 39.0386 1.13017e-07 35.1146 0 30.3251C4.13219e-05 25.5357 3.69242 21.6116 8.38573 21.2378C5.33111 17.6548 5.49497 12.2692 8.88163 8.88243C12.2683 5.49578 17.654 5.33144 21.237 8.38573C21.6108 3.69236 25.5356 0 30.3251 0Z" fill="#56C4C2" />
          <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-wink 4s ease-in-out 1.5s infinite" }}>
            <circle cx="21.356" cy="30.325" r="4.89845" stroke="#0F766E" strokeWidth="3.26563" fill="none" />
          </g>
          <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-wink 4s ease-in-out 1.5s infinite" }}>
            <circle cx="38.501" cy="30.325" r="4.89845" stroke="#0F766E" strokeWidth="3.26563" fill="none" />
          </g>
        </svg>
      </g>

      {/* Square — lavender rounded rect, float delay 1s, wink delay 3s */}
      <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-float 3s ease-in-out 1s infinite" }}>
        <svg x={(size + gap) * 2} y={0} width={size} height={size} viewBox="0 0 54.6994 54.6994">
          <rect width="54.6994" height="54.6994" rx="1.70496" ry="1.70496" fill="#988ADF" />
          <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-wink 4s ease-in-out 3s infinite" }}>
            <path d="M25.045 24.086c0 3.60712-2.9241 6.53127-6.53122 6.53127s-6.53127-2.92415-6.53127-6.53127" stroke="white" strokeWidth="3.26563" fill="none" strokeLinecap="round" />
          </g>
          <g style={{ transformBox: "fill-box", transformOrigin: "50% 50%", animation: "mascot-wink 4s ease-in-out 3s infinite" }}>
            <path d="M42.19 24.086c0 3.60712-2.9241 6.53127-6.5312 6.53127s-6.5313-2.92415-6.5313-6.53127" stroke="white" strokeWidth="3.26563" fill="none" strokeLinecap="round" />
          </g>
        </svg>
      </g>
    </svg>
  );
}
