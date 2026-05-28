import { useId, useMemo } from "react";
import { CircleUserRound, Server } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAccountMembers } from "@/api/queries/accounts";
import { UserAvatar } from "@/components/UserAvatar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { UNATTRIBUTED_USER_KEY, UNIDENTIFIED_USER_KEY, classifyUserId } from "./user-classification";

interface UsersUsedAvatarsProps {
  /** WorkOS user IDs from the blueprint's users_used field. */
  userIds: string[];
  /** Account whose member list resolves WorkOS IDs to avatars. */
  account: string;
  /** Avatars to render before collapsing into a +N overflow chip. */
  maxVisible?: number;
  className?: string;
}

export function UsersUsedAvatars({ userIds, account, maxVisible = 5, className }: UsersUsedAvatarsProps) {
  const titleId = useId();
  const { data: members } = useAccountMembers(account, { enabled: !!account });

  const memberIds = useMemo(
    () => new Set(members?.members.map((m) => m.user_id) ?? []),
    [members],
  );
  const memberById = useMemo(
    () => new Map(members?.members.map((m) => [m.user_id, m]) ?? []),
    [members],
  );

  if (userIds.length === 0) {
    return <span className="text-faint-foreground">—</span>;
  }

  const visible = userIds.slice(0, maxVisible);
  const overflow = userIds.length - visible.length;

  return (
    <div className={cn("inline-flex items-center gap-1", className)} aria-labelledby={titleId}>
      <span id={titleId} className="sr-only">
        {userIds.length} user{userIds.length === 1 ? "" : "s"}
      </span>
      <TooltipProvider delayDuration={200}>
        {visible.map((uid) => {
          const classification = classifyUserId(uid, memberIds);
          const member = memberById.get(uid);
          const label =
            classification === UNATTRIBUTED_USER_KEY ? "Infrastructure"
              : classification === UNIDENTIFIED_USER_KEY ? "Unidentified"
              : member ? (member.display_name || member.username)
              : uid;
          return (
            <Tooltip key={uid}>
              <TooltipTrigger asChild>
                <span className="inline-flex">
                  {classification === UNIDENTIFIED_USER_KEY ? (
                    <span className="inline-flex size-5 shrink-0 items-center justify-center rounded-full bg-muted">
                      <CircleUserRound className="size-3 text-muted-foreground" />
                    </span>
                  ) : classification === UNATTRIBUTED_USER_KEY ? (
                    <span className="inline-flex size-5 shrink-0 items-center justify-center rounded-full bg-muted">
                      <Server className="size-3 text-muted-foreground" />
                    </span>
                  ) : (
                    <UserAvatar
                      handle={member?.username ?? uid}
                      name={label}
                      className="size-5"
                    />
                  )}
                </span>
              </TooltipTrigger>
              <TooltipContent side="top">{label}</TooltipContent>
            </Tooltip>
          );
        })}
      </TooltipProvider>
      {overflow > 0 && (
        <span className="font-mono text-mono-sm text-muted-foreground" aria-hidden>
          +{overflow}
        </span>
      )}
    </div>
  );
}
