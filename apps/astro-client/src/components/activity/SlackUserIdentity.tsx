import type { InsightsUserIdentity } from "@/lib/api";
import { UserRound } from "lucide-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { slackIdentityDisplay } from "./insights-user-identity";

interface SlackIdentityAvatarProps {
  user: InsightsUserIdentity;
  className?: string;
  iconClassName?: string;
}

export function SlackIdentityAvatar({
  user,
  className = "size-5",
  iconClassName = "size-4",
}: SlackIdentityAvatarProps) {
  if (user.slack_avatar_url) {
    return (
      <img
        src={user.slack_avatar_url}
        alt=""
        className={cn(className, "shrink-0 rounded-full bg-muted object-cover")}
        referrerPolicy="no-referrer"
      />
    );
  }

  return (
    <span
      className={cn(className, "flex shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground")}
      aria-hidden
    >
      <UserRound className={cn(iconClassName, "shrink-0")} strokeWidth={1.75} />
    </span>
  );
}

// SlackUserIdentity renders the per-row label for an unlinked Slack user.
//
// When the server attaches a team_id via the Slack directory join, the label
// deep-links into Slack's user-profile UI (`slack://user?team=T&id=U`) so an
// admin can click through and see who the human behind the id is. Rows without
// a team_id render as plain text because the Slack URL would be incomplete.
export function SlackUserIdentity({ user, orgName }: { user: InsightsUserIdentity; orgName?: string }) {
  const display = slackIdentityDisplay(user);
  const orgLabel = orgName || "this org";
  const tooltipCopy = `This user isn't a member of ${orgLabel}.`;
  const label = (
    <span className="inline-flex min-w-0 items-center gap-2">
      <SlackIdentityAvatar
        user={user}
        className="size-5 opacity-60 transition-opacity group-hover:opacity-100"
      />
      <span
        className="truncate text-faint-foreground transition-colors group-hover:text-foreground"
        title={display.primary}
      >
        {display.primary}
      </span>
    </span>
  );

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          {display.deepLink ? (
            <a href={display.deepLink} rel="noreferrer" className="group inline-flex min-w-0 hover:underline">
              {label}
            </a>
          ) : (
            <span className="group inline-flex min-w-0">
              {label}
            </span>
          )}
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-[260px] [text-wrap:initial]">
          {tooltipCopy}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
