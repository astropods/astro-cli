"use client";

import { type ComponentPropsWithRef, forwardRef } from "react";
import { Slot } from "radix-ui";

import { ChatButton } from "@/components/assistant-ui/chat-button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export type TooltipIconButtonProps = ComponentPropsWithRef<typeof ChatButton> & {
  tooltip: string;
  side?: "top" | "bottom" | "left" | "right";
};

const chatTooltipContentClassName = "bg-foreground px-3 py-1.5 text-background";

const chatTooltipArrowClassName = "bg-foreground fill-foreground";

export const TooltipIconButton = forwardRef<
  HTMLButtonElement,
  TooltipIconButtonProps
>(({ children, tooltip, side = "bottom", className, ...rest }, ref) => {
  return (
    <TooltipProvider delayDuration={0}>
      <Tooltip>
        <TooltipTrigger asChild>
          <ChatButton
            {...rest}
            className={cn(
              // Matches the shared Button radius (see ui/button.tsx) so this size-6 icon button aligns with the app's radius scale.
              "aui-button-icon size-6 rounded-[calc(var(--radius-sm)+2px)] p-1",
              className,
            )}
            ref={ref}
          >
            <Slot.Slottable>{children}</Slot.Slottable>
            <span className="aui-sr-only sr-only">{tooltip}</span>
          </ChatButton>
        </TooltipTrigger>
        <TooltipContent
          side={side}
          className={chatTooltipContentClassName}
          arrowClassName={chatTooltipArrowClassName}
        >
          {tooltip}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
});

TooltipIconButton.displayName = "TooltipIconButton";
