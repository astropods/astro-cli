import { useMemo } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { useAccountMembers } from "@/api/queries/accounts";
import { UserAvatar } from "@/components/UserAvatar";
import { IdentityBadge } from "@/components/IdentityBadge";

export interface UserBadgeProps {
  /** WorkOS user ID (e.g. trace.user_id). */
  userId: string | null | undefined;
  /** Account whose member list maps the WorkOS ID to a profile. */
  account: string;
  /** Hide the name and render only the avatar. */
  avatarOnly?: boolean;
  /** Wrap the badge in a `<Link to="/{username}">` so clicking opens
   *  the person's profile. Only takes effect when a member is resolved. */
  linkToProfile?: boolean;
  className?: string;
}

export function UserBadge({ userId, account, avatarOnly, linkToProfile, className }: UserBadgeProps) {
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
        className={cn("truncate text-muted-foreground italic", className)}
        title={userId}
      >
        {label}
      </span>
    );
  }

  const displayName = member.display_name || member.username;

  if (avatarOnly) {
    const avatar = (
      <UserAvatar
        handle={member.username}
        name={displayName}
        className={cn("size-5", className)}
      />
    );
    return linkToProfile ? (
      <Link to={`/${member.username}`} className="inline-flex rounded-full">
        {avatar}
      </Link>
    ) : avatar;
  }

  return (
    <IdentityBadge
      avatar={<UserAvatar handle={member.username} name={displayName} className="size-5" />}
      label={displayName}
      link={linkToProfile ? { type: "internal", to: `/${member.username}` } : undefined}
      display="flex"
      className={className}
    />
  );
}
