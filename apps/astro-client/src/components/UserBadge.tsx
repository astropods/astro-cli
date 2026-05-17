import { useMemo } from "react";
import { cn } from "@/lib/utils";
import { useAccountMembers } from "@/api/queries/accounts";
import { UserAvatar } from "@/components/UserAvatar";

export interface UserBadgeProps {
  /** WorkOS user ID (e.g. trace.user_id). */
  userId: string | null | undefined;
  /** Account whose member list maps the WorkOS ID to a profile. */
  account: string;
  /** Hide the name and render only the avatar. */
  avatarOnly?: boolean;
  className?: string;
}

export function UserBadge({ userId, account, avatarOnly, className }: UserBadgeProps) {
  const { data, isLoading } = useAccountMembers(account, { enabled: !!userId && !!account });

  const member = useMemo(
    () => (userId ? data?.members.find((m) => m.user_id === userId) : undefined),
    [data, userId],
  );

  if (!userId) {
    return <span className={cn("text-muted-foreground", className)}>—</span>;
  }

  if (!member) {
    const label = isLoading ? "…" : "Unknown user";
    return (
      <span
        className={cn("truncate text-body-sm text-muted-foreground italic", className)}
        title={userId}
      >
        {label}
      </span>
    );
  }

  const displayName = member.display_name || member.username;

  if (avatarOnly) {
    return (
      <UserAvatar
        handle={member.username}
        name={displayName}
        className={cn("size-5", className)}
      />
    );
  }

  return (
    <div className={cn("flex min-w-0 items-center gap-2", className)}>
      <UserAvatar handle={member.username} name={displayName} className="size-5" />
      <span className="truncate text-body-sm text-foreground" title={displayName}>
        {displayName}
      </span>
    </div>
  );
}
