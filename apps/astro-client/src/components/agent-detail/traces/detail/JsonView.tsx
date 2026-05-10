import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneLight, oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";
import { useResolvedTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

export interface JsonViewProps {
  value: unknown;
  className?: string;
}

function safeStringify(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

/**
 * Pretty-printed JSON with Prism syntax highlighting. Matches whichever
 * Prism theme aligns with the resolved app theme.
 */
export function JsonView({ value, className }: JsonViewProps) {
  const theme = useResolvedTheme();
  const code = safeStringify(value);
  const style = theme === "dark" ? oneDark : oneLight;

  return (
    <SyntaxHighlighter
      language="json"
      style={style}
      customStyle={{
        margin: 0,
        padding: "0.75rem",
        background: "transparent",
        fontSize: "12.5px",
        lineHeight: "1.6",
      }}
      codeTagProps={{ className: cn("font-mono", className) }}
      wrapLongLines
    >
      {code}
    </SyntaxHighlighter>
  );
}
