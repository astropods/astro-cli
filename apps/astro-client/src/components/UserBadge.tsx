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
  /** Pre-resolved display name from the row's `user_details` payload.
   *  Used as the fallback when the account-member list doesn't include
   *  this user — covers cross-account spend (public-blueprint deploys)
   *  where the user belongs to a different account's member list. */
  displayName?: string;
  /** Pre-resolved username slug from `user_details.username`. Drives the
   *  profile link target when no in-account member is found. */
  username?: string;
  /** Hide the name and render only the avatar. */
  avatarOnly?: boolean;
  /** Wrap the badge in a `<Link to="/{username}">` so clicking opens
   *  the person's profile. */
  linkToProfile?: boolean;
  className?: string;
}

export function UserBadge({ userId, account, displayName: displayNameProp, username: usernameProp, avatarOnly, linkToProfile, className }: UserBadgeProps) {
  const { data, isLoading } = useAccountMembers(account, { enabled: !!userId && !!account });

  const member = useMemo(
    () => (userId ? data?.members.find((m) => m.user_id === userId) : undefined),
    [data, userId],
  );

  if (!userId) {
    return <span className={cn("text-muted-foreground", className)}>—</span>;
  }

  // Resolve display name + username with this precedence:
  //   1. account-member row (most up-to-date; carries slack workspaces etc.)
  //   2. user_details fallback (works for cross-account users)
  //   3. raw user_id (last resort)
  const resolvedUsername = member?.username ?? usernameProp;
  const resolvedDisplayName = member?.display_name || member?.username || displayNameProp || usernameProp;

  if (!resolvedUsername || !resolvedDisplayName) {
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

  const handle = resolvedUsername;
  const displayName = resolvedDisplayName;

  if (avatarOnly) {
    const avatar = (
      <UserAvatar
        handle={handle}
        name={displayName}
        className={cn("size-5", className)}
      />
    );
    return linkToProfile ? (
      <Link to={`/${handle}`} className="inline-flex rounded-full">
        {avatar}
      </Link>
    ) : avatar;
  }

  return (
    <IdentityBadge
      avatar={<UserAvatar handle={handle} name={displayName} className="size-5" />}
      label={displayName}
      link={linkToProfile ? { type: "internal", to: `/${handle}` } : undefined}
      display="flex"
      className={className}
    />
  );
}
