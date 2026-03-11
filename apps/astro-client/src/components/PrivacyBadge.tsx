import { Lock } from "lucide-react";
import { cn } from "@/lib/utils";
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
          <span
            className={cn(
              "inline-flex cursor-default items-center gap-0.5 text-xs font-normal text-muted-foreground",
              className,
            )}
          >
            <Lock className="size-3" />
            Private
          </span>
        </TooltipTrigger>
        <TooltipContent>Only visible to members with access</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
