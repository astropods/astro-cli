import { cn } from "@/lib/utils";
import { InlineBadge } from "@/components/InlineBadge";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface PrivacyBadgeProps {
  className?: string;
  onClick?: (e: React.MouseEvent) => void;
}

export function PrivacyBadge({ className, onClick }: PrivacyBadgeProps) {
  return (
    <TooltipProvider delayDuration={500}>
      <Tooltip>
        <TooltipTrigger asChild onClick={onClick}>
          <InlineBadge
            variant="soft"
            className={cn("cursor-default", className)}
            style={{
              color: "var(--color-foreground)",
              background: "color-mix(in oklch, var(--color-stone-500) 12%, transparent)",
            }}
          >
            Private
          </InlineBadge>
        </TooltipTrigger>
        <TooltipContent>Only visible to members with access</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
