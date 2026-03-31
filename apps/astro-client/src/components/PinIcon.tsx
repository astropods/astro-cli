interface PinIconProps {
  className?: string;
}

export function PinIcon({ className }: PinIconProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 16 16"
      fill="currentColor"
      className={className}
    >
      <path d="M10.97 2.22a.75.75 0 0 1 1.06 0l1.75 1.75a.75.75 0 0 1-.177 1.2l-1.268.633a.25.25 0 0 0-.119.132l-.665 1.773a.75.75 0 0 1-.182.274L9.5 9.94v1.5a.75.75 0 0 1-1.28.53L6.28 10.03l-2.5 2.5a.75.75 0 1 1-1.06-1.06l2.5-2.5L3.28 7.03a.75.75 0 0 1 .53-1.28h1.5l1.96-1.96a.75.75 0 0 1 .274-.182l1.773-.665a.25.25 0 0 0 .132-.12l.633-1.267a.75.75 0 0 1 .894-.336Z" />
    </svg>
  );
}

export function PinOutlineIcon({ className }: PinIconProps) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.3"
      className={className}
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        d="M10.97 2.22a.75.75 0 0 1 1.06 0l1.75 1.75a.75.75 0 0 1-.177 1.2l-1.268.633a.25.25 0 0 0-.119.132l-.665 1.773a.75.75 0 0 1-.182.274L9.5 9.94v1.5a.75.75 0 0 1-1.28.53L6.28 10.03l-2.5 2.5a.75.75 0 1 1-1.06-1.06l2.5-2.5L3.28 7.03a.75.75 0 0 1 .53-1.28h1.5l1.96-1.96a.75.75 0 0 1 .274-.182l1.773-.665a.25.25 0 0 0 .132-.12l.633-1.267a.75.75 0 0 1 .894-.336Z"
      />
    </svg>
  );
}
