import type { Meta, StoryObj } from "@storybook/react-vite";
import { InformationCircleIcon } from "@heroicons/react/24/outline";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

function TooltipDemo({
  label = "Hover me",
  content = "Tooltip content",
  side = "top" as "top" | "bottom" | "left" | "right",
  delayDuration = 0,
}) {
  return (
    <TooltipProvider delayDuration={delayDuration}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline">{label}</Button>
        </TooltipTrigger>
        <TooltipContent side={side}>{content}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

const meta = {
  title: "Design System/Primitives/Tooltip",
  component: TooltipDemo,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof TooltipDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: "Hover me",
    content: "This is a tooltip",
  },
};

export const Top: Story = {
  args: {
    label: "Top",
    content: "Tooltip on top",
    side: "top",
  },
};

export const Bottom: Story = {
  args: {
    label: "Bottom",
    content: "Tooltip on bottom",
    side: "bottom",
  },
};

export const Left: Story = {
  args: {
    label: "Left",
    content: "Tooltip on left",
    side: "left",
  },
};

export const Right: Story = {
  args: {
    label: "Right",
    content: "Tooltip on right",
    side: "right",
  },
};

export const WithDelay: Story = {
  name: "With Delay",
  args: {
    label: "300ms delay",
    content: "Appeared after a short delay",
    delayDuration: 300,
  },
};

export const IconTrigger: Story = {
  name: "Icon Trigger",
  render: () => (
    <TooltipProvider delayDuration={0}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon-xs">
            <InformationCircleIcon className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">More information</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  ),
};

export const AllSides: Story = {
  name: "All Sides",
  render: () => (
    <div className="flex items-center gap-4">
      <TooltipDemo label="Top" content="Top tooltip" side="top" />
      <TooltipDemo label="Bottom" content="Bottom tooltip" side="bottom" />
      <TooltipDemo label="Left" content="Left tooltip" side="left" />
      <TooltipDemo label="Right" content="Right tooltip" side="right" />
    </div>
  ),
};
