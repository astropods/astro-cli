import { useId, useMemo } from "react";
import { Link } from "react-router";
import { CircleUserRound, Server } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAccountMembers } from "@/api/queries/accounts";
import type { AccountMember } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { UNATTRIBUTED_USER_KEY, UNIDENTIFIED_USER_KEY, classifyUserId } from "./user-classification";
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
 *  both the visible-avatars row and the +N overflow popover. */
interface ClassifiedUser {
  uid: string;
  classification: ReturnType<typeof classifyUserId>;
  member: AccountMember | undefined;
  /** Display string for the row label, popover line, and avatar tooltip. */
  primary: string;
  /** True when the row should link to `/{username}`. Buckets stay text. */
  isMember: boolean;
}

function classify(
  uid: string,
  memberById: Map<string, AccountMember>,
  memberIds: Set<string>,
): ClassifiedUser {
  const member = memberById.get(uid);
  const classification = classifyUserId(uid, memberIds);
  const primary =
    classification === UNATTRIBUTED_USER_KEY ? "System spend"
      : classification === UNIDENTIFIED_USER_KEY ? `Slack ID: ${uid}`
      : member ? (member.display_name || member.username)
      : uid;
  const isMember =
    classification !== UNIDENTIFIED_USER_KEY &&
    classification !== UNATTRIBUTED_USER_KEY &&
    !!member;
  return { uid, classification, member, primary, isMember };
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
  // logic lives in one place. The member-id Set is built once per render
  // (not once per uid) since classifyUserId needs Set lookup.
  const classified = useMemo<ClassifiedUser[]>(() => {
    const memberIds = new Set(memberById.keys());
    return userIds.map((uid) => classify(uid, memberById, memberIds));
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
            c.classification === UNIDENTIFIED_USER_KEY ? (
              <CircleUserRound className="size-5 shrink-0 text-muted-foreground" aria-hidden />
            ) : c.classification === UNATTRIBUTED_USER_KEY ? (
              <Server className="size-5 shrink-0 text-muted-foreground" aria-hidden />
            ) : (
              <UserAvatar
                handle={c.member?.username ?? c.uid}
                name={c.primary}
                className="size-5"
              />
            );
          return (
            <Tooltip key={c.uid}>
              <TooltipTrigger asChild>
                {c.isMember && c.member ? (
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
                  {c.classification === UNIDENTIFIED_USER_KEY ? (
                    <CircleUserRound className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                  ) : c.classification === UNATTRIBUTED_USER_KEY ? (
                    <Server className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                  ) : (
                    <UserAvatar
                      handle={c.member?.username ?? c.uid}
                      name={c.primary}
                      className="size-4 shrink-0"
                    />
                  )}
                  <span className="min-w-0 truncate">{c.primary}</span>
                </>
              );
              return (
                <li key={c.uid}>
                  {c.isMember && c.member ? (
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
