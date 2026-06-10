import type { CSSProperties } from "react";
import { Link } from "react-router";
import {
  ArrowRightIcon,
  ChatBubbleLeftRightIcon,
} from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { AstroLogoLoader } from "@/components/ui/astro-logo-loader";
import { AgentMascots } from "@/components/AgentMascots";
import { blueprintsAccountPath } from "@/lib/routes";
import { cn } from "@/lib/utils";

const emptyCardStyle: CSSProperties = {
  backgroundImage:
    "radial-gradient(ellipse 120% 80% at 50% 0%, color-mix(in oklch, var(--muted) 55%, transparent) 0%, transparent 55%), radial-gradient(ellipse 90% 70% at 80% 100%, color-mix(in oklch, var(--primary) 10%, transparent) 0%, transparent 50%)",
};

export function ChatEmptyState({
  variant,
  account,
  className,
}: {
  variant: "loading" | "no-chat-agents";
  account: string;
  className?: string;
}) {
  if (variant === "loading") {
    return (
      <div
        className={cn(
          "flex min-h-0 flex-col items-center justify-center gap-6 bg-background px-6 py-16",
          className,
        )}
      >
        <AstroLogoLoader size={72} />
        <div className="text-center">
          <p className="text-sm font-medium text-foreground">
            Loading chat agents
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Checking which deployments are ready for messaging…
          </p>
        </div>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex min-h-0 items-center justify-center overflow-y-auto bg-background p-6 md:p-10",
        className,
      )}
    >
      <div
        className="flex w-full max-w-md flex-col items-center rounded-xl border border-border bg-background px-6 py-12 text-center shadow-sm"
        style={emptyCardStyle}
      >
        <div className="mb-4 flex size-14 items-center justify-center rounded-full bg-muted">
          <ChatBubbleLeftRightIcon
            className="size-7 text-muted-foreground"
            aria-hidden
          />
        </div>
        <div className="mb-5">
          <AgentMascots size={28} />
        </div>
        <h2 className="mb-2 text-lg font-semibold tracking-tight text-foreground">
          No agents ready for chat
        </h2>
        <p className="mb-6 max-w-sm text-sm leading-relaxed text-muted-foreground">
          Chat works with deployed agents that have{" "}
          <span className="text-foreground">web messaging</span> enabled. Deploy
          or update a blueprint with a messaging HTTP interface, then come back
          here to talk to your agent.
        </p>
        <Button asChild>
          <Link to={blueprintsAccountPath(account)} className="gap-1.5">
            View your blueprints
            <ArrowRightIcon className="size-4" />
          </Link>
        </Button>
      </div>
    </div>
  );
}
