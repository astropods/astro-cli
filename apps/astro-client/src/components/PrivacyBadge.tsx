import { Lock } from "lucide-react";
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
          <InlineBadge className={cn("cursor-default gap-1", className)}>
            <Lock className="size-3" />
            Private
          </InlineBadge>
        </TooltipTrigger>
        <TooltipContent>Only visible to members with access</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
