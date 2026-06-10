import { useId, useMemo, type ReactNode } from "react";
import { Link } from "react-router";
import { Server, User } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAccountMembers } from "@/api/queries/accounts";
import type { AccountMember, InsightsUserIdentity } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { isSlackUserId } from "./user-classification";
import { OverflowPopover } from "./OverflowPopover";
import { SlackIdentityAvatar } from "./SlackUserIdentity";
import {
  identityRefFromUserID,
  insightsUserIdentityKey,
  slackIdentityDisplay,
} from "./insights-user-identity";

interface UsersUsedAvatarsProps {
  /** Legacy IDs from the deployment's users_used field. */
  userIds: string[];
  /** Rich identities from users_used_details. Preferred when present. */
  users?: InsightsUserIdentity[];
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
  key: string;
  identity: InsightsUserIdentity;
  kind: UserKind;
  member: AccountMember | undefined;
  /** Display string for the row label, popover line, and avatar tooltip. */
  primary: string;
  secondary?: string;
  deepLink?: string;
}

function classify(identity: InsightsUserIdentity, member: AccountMember | undefined): ClassifiedUser {
  const uid = identity.user_id;
  const key = insightsUserIdentityKey(identity);
  if (!uid) {
    return { key, identity, kind: "unattributed", member: undefined, primary: "System spend" };
  }
  if (member) {
    return {
      key,
      identity,
      kind: "member",
      member,
      primary: member.display_name || member.username,
      secondary: `@${member.username}`,
    };
  }
  if (isSlackUserId(uid)) {
    const display = slackIdentityDisplay(identity);
    return {
      key,
      identity,
      kind: "slack",
      member: undefined,
      primary: display.primary,
      secondary: identity.slack_username ? `@${identity.slack_username}` : undefined,
      deepLink: display.deepLink,
    };
  }
  return { key, identity, kind: "unidentified", member: undefined, primary: uid };
}

function UserChipAvatar({ user }: { user: ClassifiedUser }) {
  if (user.kind === "slack") {
    return (
      <SlackIdentityAvatar
        user={user.identity}
        className="size-6 opacity-60 transition-opacity group-hover:opacity-100"
        iconClassName="size-3.5"
      />
    );
  }
  if (user.kind === "unidentified") {
    return (
      <span
        className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
        aria-hidden
      >
        <User className="size-3.5" strokeWidth={1.75} />
      </span>
    );
  }
  if (user.kind === "unattributed") {
    return <Server className="size-6 shrink-0 text-faint-foreground" aria-hidden />;
  }
  return (
    <UserAvatar
      handle={user.member?.username ?? user.identity.user_id}
      name={user.primary}
      className="size-6 shrink-0"
    />
  );
}

function userTooltipTitle(user: ClassifiedUser) {
  return user.secondary ? `${user.primary} (${user.secondary})` : user.primary;
}

function UserTooltipContent({ user }: { user: ClassifiedUser }) {
  return (
    <>
      <span className="block">{user.primary}</span>
      {user.secondary && (
        <span className="block text-faint-foreground">{user.secondary}</span>
      )}
    </>
  );
}

function userIdentityTarget(
  user: ClassifiedUser,
  className: string,
  children: ReactNode,
  options?: { title?: boolean },
) {
  const title = options?.title === false ? undefined : userTooltipTitle(user);
  if (user.kind === "member" && user.member) {
    return (
      <Link to={`/${user.member.username}`} className={className} title={title}>
        {children}
      </Link>
    );
  }
  if (user.kind === "slack" && user.deepLink) {
    return (
      <a href={user.deepLink} rel="noreferrer" className={className} title={title}>
        {children}
      </a>
    );
  }
  return <span className={className} title={title}>{children}</span>;
}

export function UsersUsedAvatars({
  userIds,
  users,
  account,
  maxVisible = 3,
  className,
}: UsersUsedAvatarsProps) {
  const titleId = useId();
  const { data: members } = useAccountMembers(account, { enabled: !!account });

  const memberById = useMemo(
    () => new Map(members?.members.map((m) => [m.user_id, m]) ?? []),
    [members],
  );

  const identityRefs = useMemo(
    () => users && users.length > 0 ? users : userIds.map(identityRefFromUserID),
    [users, userIds],
  );

  // Single pass over the identities: both the visible chips and the +N
  // popover read from this list so the classification + name derivation
  // logic lives in one place.
  const classified = useMemo<ClassifiedUser[]>(() => {
    return identityRefs.map((identity) => classify(identity, memberById.get(identity.user_id)));
  }, [identityRefs, memberById]);

  if (identityRefs.length === 0) {
    return <span className="text-faint-foreground">—</span>;
  }

  const visible = classified.slice(0, maxVisible);
  const overflow = classified.length - visible.length;

  return (
    <div className={cn("inline-flex items-center gap-1", className)} aria-labelledby={titleId}>
      <span id={titleId} className="sr-only">
        {identityRefs.length} user{identityRefs.length === 1 ? "" : "s"}
      </span>
      <TooltipProvider delayDuration={200}>
        {visible.map((c) => {
          const avatarNode = <UserChipAvatar user={c} />;
          const target = userIdentityTarget(
            c,
            cn(
              "inline-flex rounded-full",
              (c.kind === "slack" || c.kind === "unattributed") && "group",
            ),
            avatarNode,
            { title: false },
          );
          return (
            <Tooltip key={c.key}>
              <TooltipTrigger asChild>
                {target}
              </TooltipTrigger>
              <TooltipContent side="top">
                <UserTooltipContent user={c} />
              </TooltipContent>
            </Tooltip>
          );
        })}
      </TooltipProvider>
      {overflow > 0 && (
        <OverflowPopover
          overflow={overflow}
          total={identityRefs.length}
          itemNoun={{ singular: "person", plural: "people" }}
        >
          <ul className="min-h-0 flex-1 space-y-0.5 overflow-y-auto">
            {classified.map((c) => {
              const isDimmed = c.kind === "slack" || c.kind === "unattributed";
              const rowBody = (
                <>
                  <UserChipAvatar user={c} />
                  <span className="min-w-0">
                    <span
                      className={cn(
                        "block truncate transition-colors",
                        isDimmed && "text-faint-foreground group-hover:text-foreground",
                        c.kind === "unidentified" && "font-mono",
                      )}
                    >
                      {c.primary}
                    </span>
                  </span>
                </>
              );
              return (
                <li key={c.key}>
                  {userIdentityTarget(
                    c,
                    cn(
                      "group flex items-center gap-2 rounded px-2 py-1 text-body-sm text-foreground",
                      isDimmed && "text-faint-foreground",
                      (c.kind === "member" || c.deepLink) && "hover:bg-muted",
                    ),
                    rowBody,
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
