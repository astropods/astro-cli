import type { UserDetails } from "@/lib/api";
import { UserBadge } from "@/components/UserBadge";
import { SlackUserIdentity } from "@/components/activity/SlackUserIdentity";
import { identityRefFromUserID } from "@/components/activity/insights-user-identity";

interface TraceUserIdentityProps {
  userId?: string;
  userDetails?: UserDetails;
  account: string;
}

export function TraceUserIdentity({ userId, userDetails, account }: TraceUserIdentityProps) {
  if (!userId) {
    return <span className="text-muted-foreground">—</span>;
  }

  const identity = userDetails
    ? { user_id: userId, user_details: userDetails }
    : identityRefFromUserID(userId);

  if (identity.user_details.kind === "slack") {
    return <SlackUserIdentity user={identity} variant="trace" />;
  }

  return <UserBadge userId={userId} account={account} className="max-w-full" />;
}
