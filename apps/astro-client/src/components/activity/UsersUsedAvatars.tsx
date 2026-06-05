import { useId, useMemo } from "react";
import { Link } from "react-router";
import { Server, Slack, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAccountMembers } from "@/api/queries/accounts";
import type { AccountMember } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { isSlackUserId } from "./user-classification";
import { OverflowPopover } from "./OverflowPopover";

interface UsersUsedAvatarsProps {
  /** WorkOS user IDs from the blueprint's users_used field. */
  userIds: string[];
  /** Account whose member list resolves WorkOS IDs to avatars. */
  account: string;
  /** Avatars to render before collapsing into a +N overflow chip. */
  maxVisible?: number;
  className?: string;
}

/** Per-uid classification result — derived once from members, consumed by
 *  both the visible-avatars row and the +N overflow popover. The `kind`
 *  drives avatar selection (member chip / Slack icon / generic / system),
 *  the row label, and whether the chip links to a profile. */
type UserKind = "member" | "slack" | "unidentified" | "unattributed";

interface ClassifiedUser {
  uid: string;
  kind: UserKind;
  member: AccountMember | undefined;
  /** Display string for the row label, popover line, and avatar tooltip. */
  primary: string;
}

function classify(uid: string, member: AccountMember | undefined): ClassifiedUser {
  if (!uid) {
    return { uid, kind: "unattributed", member: undefined, primary: "System spend" };
  }
  if (member) {
    return { uid, kind: "member", member, primary: member.display_name || member.username };
  }
  if (isSlackUserId(uid)) {
    // Matches the per-row label rendered by SlackUserIdentity in
    // TopSpendersTable so the People column on the agents view reads the
    // same way as the People table on Insights.
    return { uid, kind: "slack", member: undefined, primary: `Slack user - ${uid}` };
  }
  return { uid, kind: "unidentified", member: undefined, primary: uid };
}

export function UsersUsedAvatars({ userIds, account, maxVisible = 3, className }: UsersUsedAvatarsProps) {
  const titleId = useId();
  const { data: members } = useAccountMembers(account, { enabled: !!account });

  const memberById = useMemo(
    () => new Map(members?.members.map((m) => [m.user_id, m]) ?? []),
    [members],
  );

  // Single pass over the userIds — both the visible chips and the +N
  // popover read from this list so the classification + name derivation
  // logic lives in one place.
  const classified = useMemo<ClassifiedUser[]>(() => {
    return userIds.map((uid) => classify(uid, memberById.get(uid)));
  }, [userIds, memberById]);

  if (userIds.length === 0) {
    return <span className="text-faint-foreground">—</span>;
  }

  const visible = classified.slice(0, maxVisible);
  const overflow = classified.length - visible.length;

  return (
    <div className={cn("inline-flex items-center gap-1", className)} aria-labelledby={titleId}>
      <span id={titleId} className="sr-only">
        {userIds.length} user{userIds.length === 1 ? "" : "s"}
      </span>
      <TooltipProvider delayDuration={200}>
        {visible.map((c) => {
          const avatarNode =
            c.kind === "slack" ? (
              <Slack className="size-6 shrink-0 text-muted-foreground" aria-hidden />
            ) : c.kind === "unidentified" ? (
              <span
                className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
                aria-hidden
              >
                <User className="size-3.5" strokeWidth={1.75} />
              </span>
            ) : c.kind === "unattributed" ? (
              <Server className="size-6 shrink-0 text-muted-foreground" aria-hidden />
            ) : (
              <UserAvatar
                handle={c.member?.username ?? c.uid}
                name={c.primary}
                className="size-6"
              />
            );
          return (
            <Tooltip key={c.uid}>
              <TooltipTrigger asChild>
                {c.kind === "member" && c.member ? (
                  <Link to={`/${c.member.username}`} className="inline-flex rounded-full">
                    {avatarNode}
                  </Link>
                ) : (
                  <span className="inline-flex">{avatarNode}</span>
                )}
              </TooltipTrigger>
              <TooltipContent side="top">{c.primary}</TooltipContent>
            </Tooltip>
          );
        })}
      </TooltipProvider>
      {overflow > 0 && (
        <OverflowPopover
          overflow={overflow}
          total={userIds.length}
          itemNoun={{ singular: "person", plural: "people" }}
        >
          <ul className="min-h-0 flex-1 space-y-0.5 overflow-y-auto">
            {classified.map((c) => {
              const rowBody = (
                <>
                  {c.kind === "slack" ? (
                    <Slack className="size-6 shrink-0 text-muted-foreground" aria-hidden />
                  ) : c.kind === "unidentified" ? (
                    <span
                      className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
                      aria-hidden
                    >
                      <User className="size-3.5" strokeWidth={1.75} />
                    </span>
                  ) : c.kind === "unattributed" ? (
                    <Server className="size-6 shrink-0 text-muted-foreground" aria-hidden />
                  ) : (
                    <UserAvatar
                      handle={c.member?.username ?? c.uid}
                      name={c.primary}
                      className="size-6 shrink-0"
                    />
                  )}
                  <span
                    className={cn(
                      "min-w-0 truncate",
                      c.kind === "unidentified" && "font-mono text-mono-sm",
                    )}
                  >
                    {c.primary}
                  </span>
                </>
              );
              return (
                <li key={c.uid}>
                  {c.kind === "member" && c.member ? (
                    <Link
                      to={`/${c.member.username}`}
                      className="flex items-center gap-2 rounded px-2 py-1 text-body-sm text-foreground hover:bg-muted"
                    >
                      {rowBody}
                    </Link>
                  ) : (
                    <span className="flex items-center gap-2 rounded px-2 py-1 text-body-sm text-foreground">
                      {rowBody}
                    </span>
                  )}
                </li>
              );
            })}
          </ul>
        </OverflowPopover>
      )}
    </div>
  );
}
