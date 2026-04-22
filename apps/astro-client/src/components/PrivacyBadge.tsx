import { cn } from "@/lib/utils";
import { Tag } from "@/components/Tag";
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
          <Tag color="accent" className={cn("cursor-default", className)}>Private</Tag>
        </TooltipTrigger>
        <TooltipContent>Only visible to members with access</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
