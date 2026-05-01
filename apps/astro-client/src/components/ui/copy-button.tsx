import { Square2StackIcon, CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

interface CopyButtonProps {
  /** Static string or function that returns the text to copy at click time. */
  copyText: string | (() => string);
  title?: string;
  resetMs?: number;
  iconClassName?: string;
  className?: string;
}

export function CopyButton({ copyText, title = "Copy", resetMs, iconClassName = "size-4", className }: CopyButtonProps) {
  const { copy, copied } = useCopyToClipboard(resetMs);

  return (
    <Button
      variant="outline"
      size="icon"
      title={copied ? "Copied!" : title}
      onClick={() => {
        const text = typeof copyText === "function" ? copyText() : copyText;
        void copy(text);
      }}
      className={className}
    >
      {copied ? <CheckIcon className={cn("text-success", iconClassName)} /> : <Square2StackIcon className={iconClassName} />}
    </Button>
  );
}
