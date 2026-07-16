import { ArrowLeftRight, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";

const ANNOUNCEMENT = "Switch agents here";

/**
 * First-run nudge anchored under the chat agent switcher. Rendered inside the
 * relative chat header; positioned just below the switcher trigger with a notch
 * pointing up at it. Dismissal (and one-time persistence) is owned by the host.
 */
export function ChatAgentSwitchCoachmark({ onClose }: { onClose: () => void }) {
  // Live regions only announce text inserted after they mount, so fill it next
  // frame rather than mounting the region already populated.
  const [announcement, setAnnouncement] = useState("");
  useEffect(() => {
    const frame = requestAnimationFrame(() => setAnnouncement(ANNOUNCEMENT));
    return () => cancelAnimationFrame(frame);
  }, []);

  return (
    <div className="coachmark-enter absolute left-3 top-full z-20 mt-2 md:left-4">
      <span role="status" aria-live="polite" className="sr-only">
        {announcement}
      </span>
      <div className="coachmark-bob relative flex items-center gap-2.5 rounded-md border border-border bg-popover py-2 pl-3 pr-2 text-body text-foreground shadow-lg">
        <span
          aria-hidden
          className="absolute -top-1 left-5 size-2 rotate-45 rounded-[2px] border-l border-t border-border bg-popover"
        />
        <ArrowLeftRight className="size-4 shrink-0 text-foreground-accent" />
        <span className="whitespace-nowrap font-medium">
          Switch agents here
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label="Dismiss"
          onClick={onClose}
          className="-mr-0.5 ml-1 text-muted-foreground hover:text-foreground"
        >
          <X className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}
