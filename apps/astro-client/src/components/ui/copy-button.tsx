import { Copy, Check } from "lucide-react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

interface CopyButtonProps {
  /** Static string or function that returns the text to copy at click time. */
  copyText: string | (() => string);
  title?: string;
  resetMs?: number;
  size?: number;
  className?: string;
}

export function CopyButton({ copyText, title = "Copy", resetMs, size = 12, className }: CopyButtonProps) {
  const { copy, copied } = useCopyToClipboard(resetMs);

  return (
    <button
      type="button"
      title={copied ? "Copied!" : title}
      onClick={() => {
        const text = typeof copyText === "function" ? copyText() : copyText;
        void copy(text);
      }}
      className={cn(
        "flex items-center justify-center size-8 rounded border border-border bg-transparent text-foreground cursor-pointer",
        className,
      )}
    >
      {copied ? <Check size={size} className="text-teal-600" /> : <Copy size={size} />}
    </button>
  );
}
