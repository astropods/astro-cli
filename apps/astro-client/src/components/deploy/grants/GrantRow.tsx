import { X, Globe, User as UserIcon } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import { SlackUnlinkedBadge } from "./SlackUnlinkedBadge";
import type { Account, AccountMember, AuthGrant } from "@/lib/api";

export interface GrantRowProps {
  grant: AuthGrant;
  /** Adapter this grant belongs to. Drives the "no Slack connection" warning. */
  adapter: "web" | "slack" | "custom";
  /** Map of account ID → account for resolving account-scope grants (handle + display name). */
  accountById: Map<string, Account>;
  /** Map of user ID → member record for resolving user-scope grants. */
  memberByUserId: Map<string, AccountMember>;
  onRemove: () => void;
  /** The deploying user's id. Their own grant is marked "(you)" and locked (no remove). */
  currentUserId?: string;
}

export function GrantRow({ grant, adapter, accountById, memberByUserId, onRemove, currentUserId }: GrantRowProps) {
  const { primary, secondary } = grantText(grant, adapter, accountById, memberByUserId);
  const isSelf = !!currentUserId && grant.user_id === currentUserId;
  const member = grant.user_id ? memberByUserId.get(grant.user_id) : undefined;
  // The mapping between WorkOS users and Slack accounts is per-workspace, so
  // a user with zero linked workspaces can't be resolved at request time no
  // matter which workspace the message arrives from.
  const slackUnlinked =
    adapter === "slack" && !!member && (member.slack_workspaces?.length ?? 0) === 0;
  return (
    <li className="flex items-center justify-between gap-2 rounded-[4px] border border-border bg-card px-2.5 py-1.5">
      <div className="flex items-center gap-2 min-w-0">
        <GrantBadge grant={grant} accountById={accountById} memberByUserId={memberByUserId} />
        <span className="flex flex-col min-w-0">
          <span className="flex items-center gap-1.5 min-w-0">
            <span className="text-[13px] text-foreground truncate">
              {primary}
              {isSelf && <span className="text-muted-foreground"> (you)</span>}
            </span>
            {slackUnlinked && <SlackUnlinkedBadge />}
          </span>
          {secondary && (
            <span className="text-[11px] text-muted-foreground truncate">{secondary}</span>
          )}
        </span>
      </div>
      {!isSelf && (
        <button
          type="button"
          aria-label="Remove grant"
          onClick={onRemove}
          className="text-muted-foreground hover:text-destructive shrink-0 cursor-pointer"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      )}
    </li>
  );
}

/** Picks the right glyph for the grant: avatar for resolved subjects, globe for "Anyone",
 *  plain icon as a fallback when an account/user can't be resolved. */
function GrantBadge({
  grant,
  accountById,
  memberByUserId,
}: {
  grant: AuthGrant;
  accountById: Map<string, Account>;
  memberByUserId: Map<string, AccountMember>;
}) {
  if (grant.anyone) {
    return (
      <span className="size-6 rounded-full bg-primary/10 flex items-center justify-center text-primary shrink-0">
        <Globe className="h-3.5 w-3.5" />
      </span>
    );
  }
  if (grant.org) {
    const a = accountById.get(grant.org);
    if (a) return <UserAvatar handle={a.name} name={a.display_name ?? a.name} avatarUrl={a.avatar_url} className="size-6 rounded-sm" />;
    return <FallbackIcon />;
  }
  if (grant.user_id) {
    const m = memberByUserId.get(grant.user_id);
    if (m) return <UserAvatar handle={m.username} name={m.display_name || m.username} avatarUrl={m.avatar_url} className="size-6" />;
    return <FallbackIcon />;
  }
  return <FallbackIcon />;
}

function FallbackIcon() {
  return (
    <span className="size-6 rounded-full bg-stone-200 dark:bg-stone-800 flex items-center justify-center text-muted-foreground shrink-0">
      <UserIcon className="h-3.5 w-3.5" />
    </span>
  );
}

function grantText(
  g: AuthGrant,
  adapter: "web" | "slack" | "custom",
  accountById: Map<string, Account>,
  memberByUserId: Map<string, AccountMember>,
): { primary: string; secondary?: string } {
  if (g.anyone) {
    // Web/custom go through the OIDC gate, so "anyone" means any authenticated
    // Astro account. Slack's "anyone" is workspace-scoped, not account-scoped.
    return adapter === "slack"
      ? { primary: "Anyone", secondary: "Any workspace member" }
      : { primary: "Anyone with an Astro account" };
  }
  if (g.org) {
    const a = accountById.get(g.org);
    return a
      ? { primary: a.display_name ?? a.name, secondary: `Members of @${a.name}` }
      : { primary: `Org ${g.org}` };
  }
  if (g.user_id) {
    const m = memberByUserId.get(g.user_id);
    if (m) {
      return {
        primary: m.display_name || m.username,
        secondary: `@${m.username}`,
      };
    }
    return { primary: g.user_id };
  }
  return { primary: "Unknown" };
}
