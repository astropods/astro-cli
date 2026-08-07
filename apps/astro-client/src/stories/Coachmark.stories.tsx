import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ArrowLeftRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Coachmark, CoachmarkSurface } from "@/components/ui/coachmark";

function CoachmarkDemo() {
  const [open, setOpen] = useState(true);

  return (
    <Coachmark
      open={open}
      anchor={
        <Button variant="outline" onClick={() => setOpen(true)}>
          Switch agents
        </Button>
      }
      sideOffset={8}
      announcement="Switch agents here"
      contentClassName="flex items-center gap-2.5 py-2 pl-3 pr-2 text-body text-foreground"
    >
      <ArrowLeftRight className="size-4 shrink-0 text-foreground-accent" />
      <span className="whitespace-nowrap font-medium">Switch agents here</span>
      <Button
        variant="ghost"
        size="xs"
        onClick={() => setOpen(false)}
        className="ml-1"
      >
        Got it
      </Button>
    </Coachmark>
  );
}

const meta = {
  title: "Design System/Primitives/Coachmark",
  component: Coachmark,
  args: {
    open: true,
    anchor: null,
    children: null,
  },
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof Coachmark>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Anchored: Story = {
  render: () => <CoachmarkDemo />,
};

export const StaticSurface: Story = {
  name: "Static surface",
  render: () => (
    <CoachmarkSurface className="w-2xs rounded-2xl p-4 shadow-xl">
      <h2 className="text-heading-4 font-semibold text-foreground">
        Panel heading
      </h2>
      <p className="mt-2 text-body-sm text-muted-foreground">
        The same notched surface without the coachmark entrance or bob, for
        hover guidance panels.
      </p>
    </CoachmarkSurface>
  ),
};
