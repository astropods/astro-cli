import * as React from "react";
import { Slider as SliderPrimitive } from "radix-ui";

import { cn } from "@/lib/utils";

function Slider({
  className,
  ...props
}: React.ComponentProps<typeof SliderPrimitive.Root>) {
  return (
    <SliderPrimitive.Root
      data-slot="slider"
      className={cn(
        "relative flex w-full touch-none select-none items-center",
        "data-[disabled]:opacity-40",
        className,
      )}
      {...props}
    >
      <SliderPrimitive.Track
        data-slot="slider-track"
        className="relative h-1.5 w-full grow overflow-hidden rounded-full bg-border"
      >
        <SliderPrimitive.Range
          data-slot="slider-range"
          className="absolute h-full bg-foreground transition-[left,right] duration-200 ease-[cubic-bezier(0.32,0.72,0,1)]"
        />
      </SliderPrimitive.Track>
      <SliderPrimitive.Thumb
        data-slot="slider-thumb"
        className={cn(
          "block size-4 rounded-full border border-foreground bg-card shadow-sm",
          "transition-[left,right,background-color,transform] duration-200 ease-[cubic-bezier(0.32,0.72,0,1)]",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          "hover:bg-popover active:scale-110",
          "data-[disabled]:hover:bg-card data-[disabled]:active:scale-100",
        )}
      />
    </SliderPrimitive.Root>
  );
}

export { Slider };
