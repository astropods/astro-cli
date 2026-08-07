import {
  useEffect,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { cn } from "@/lib/utils";
import {
  Popover,
  PopoverAnchor,
  PopoverPositioner,
} from "@/components/ui/popover";

export function CoachmarkSurface({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "relative rounded-md border border-border bg-popover text-popover-foreground shadow-lg",
        className,
      )}
    >
      <span
        aria-hidden
        className="absolute -top-1 left-5 size-2 rotate-45 rounded-[2px] border-l border-t border-border bg-popover"
      />
      {children}
    </div>
  );
}

function PoliteAnnouncement({ text }: { text: string }) {
  // Live regions only announce text inserted after they mount, so mount empty
  // and fill on the next frame.
  const [announced, setAnnounced] = useState("");

  useEffect(() => {
    const frame = requestAnimationFrame(() => setAnnounced(text));
    return () => cancelAnimationFrame(frame);
  }, [text]);

  return (
    <span role="status" aria-live="polite" className="sr-only">
      {announced}
    </span>
  );
}

export function Coachmark({
  open,
  anchor,
  children,
  announcement,
  className,
  contentClassName,
  side = "bottom",
  align = "start",
  ...positionerProps
}: {
  open: boolean;
  anchor: ReactNode;
  children: ReactNode;
  announcement?: string;
  className?: string;
  contentClassName?: string;
} & Omit<ComponentProps<typeof PopoverPositioner>, "children">) {
  return (
    <Popover open={open}>
      <PopoverAnchor asChild>
        <span className="inline-flex">{anchor}</span>
      </PopoverAnchor>
      <PopoverPositioner
        className={className}
        side={side}
        align={align}
        onOpenAutoFocus={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => event.preventDefault()}
        onPointerDownOutside={(event) => event.preventDefault()}
        {...positionerProps}
      >
        <div className="coachmark-enter">
          {announcement && <PoliteAnnouncement text={announcement} />}
          <CoachmarkSurface className={cn("coachmark-bob", contentClassName)}>
            {children}
          </CoachmarkSurface>
        </div>
      </PopoverPositioner>
    </Popover>
  );
}
