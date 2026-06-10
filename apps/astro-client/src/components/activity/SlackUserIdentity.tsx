import type { InsightsUserIdentity } from "@/lib/api";
import { cn } from "@/lib/utils";
import { IdentityBadge } from "@/components/IdentityBadge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Slack } from "@/components/ui/svgs/slack";
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
      <Slack className={cn(iconClassName, "shrink-0")} />
    </span>
  );
}

// SlackUserIdentity renders the per-row label for an unlinked Slack user.
//
// When the server attaches a team_id via the Slack directory join, the label
// deep-links into Slack's user-profile UI (`slack://user?team=T&id=U`) so an
// admin can click through and see who the human behind the id is. Rows without
// a team_id render as plain text because the Slack URL would be incomplete.
export function SlackUserIdentity({ user }: { user: InsightsUserIdentity }) {
  const display = slackIdentityDisplay(user);

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <IdentityBadge
            avatar={<SlackIdentityAvatar user={user} />}
            label={display.primary}
            link={display.deepLink ? { type: "external", href: display.deepLink, rel: "noreferrer" } : undefined}
            className="inline-flex text-body-sm text-foreground"
          />
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-[260px] [text-wrap:initial]">
          {display.deepLink
            ? "Slack user not linked to an Astro account. Click to open their Slack profile."
            : "Slack user not linked to an Astro account. Connect to attribute their usage to a member."}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
