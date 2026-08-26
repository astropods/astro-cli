import type { ReactNode } from "react";

const EMAIL = /([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/g;

export function linkifyEmail(text: string): ReactNode {
  const parts = text.split(EMAIL);
  if (parts.length === 1) return text;
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <a key={i} href={`mailto:${part}`} className="underline underline-offset-2">
        {part}
      </a>
    ) : (
      part
    ),
  );
}
