import { Link } from "react-router";
import type { AccountPublic, AccountOrg } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { detectSocialLink, type SocialLinkDisplay } from "@/lib/social-links";
import { EarlyAdopterBadge } from "@/components/account-profile/EarlyAdopterBadge";
import { BotIcon, LayersIcon, Calendar, Globe, MapPin, Mail, User2 } from "lucide-react";

// ── MetaRow ────────────────────────────────────────────────────────────────────
// Icon + text row used throughout the profile sidebar.
// Pass href for clickable rows (handles mailto: internally).

function MetaRow({
  icon,
  children,
  href,
}: {
  icon: React.ReactNode;
  children: React.ReactNode;
  href?: string;
}) {
  const base = "flex items-center gap-2 text-[13px] leading-none text-muted-foreground";
  const content = (
    <>
      <span className="shrink-0">{icon}</span>
      <span className="truncate">{children}</span>
    </>
  );
  if (!href) return <div className={base}>{content}</div>;
  if (href.startsWith("/")) {
    return <Link to={href} className={`${base} transition-colors hover:text-foreground`}>{content}</Link>;
  }
  return (
    <a
      href={href}
      target={href.startsWith("mailto:") ? undefined : "_blank"}
      rel={href.startsWith("mailto:") ? undefined : "noopener noreferrer"}
      className={`${base} transition-colors hover:text-foreground`}
    >
      {content}
    </a>
  );
}

// ── ProfileViewSidebar ─────────────────────────────────────────────────────────

interface ProfileViewSidebarProps {
  data: AccountPublic;
  isOwner: boolean;
  blueprintCount: number;
  deploymentCount: number;
  orgs: AccountOrg[];
  onEditOpen?: () => void;
}

export function ProfileViewSidebar({
  data,
  isOwner,
  blueprintCount,
  deploymentCount,
  orgs,
  onEditOpen,
}: ProfileViewSidebarProps) {
  const displayName = data.display_name || data.name;
  const joinedDate = new Date(data.created_at).toLocaleDateString("en-US", {
    month: "short",
    year: "numeric",
  });

  const socialLinks = (data.social_links ?? [])
    .map((v) => detectSocialLink(v))
    .filter((x): x is SocialLinkDisplay => x !== null);

  const websiteHref = data.website
    ? /^https?:\/\//.test(data.website)
      ? data.website
      : `https://${data.website}`
    : null;
  const websiteLabel = websiteHref
    ? (() => {
        try {
          return new URL(websiteHref).hostname.replace(/^www\./, "");
        } catch {
          return data.website;
        }
      })()
    : null;

  const stats = [
    { label: "Blueprints", value: blueprintCount, icon: <LayersIcon className="size-3.5" /> },
    ...(isOwner
      ? [{ label: "Agents", value: deploymentCount, icon: <BotIcon className="size-3.5" /> }]
      : []),
  ];

  return (
    <div className="relative z-10 flex flex-col gap-6 px-6 py-7 h-full overflow-y-auto">
      {/* Avatar + identity */}
      <div className="flex flex-col gap-2">
        <UserAvatar handle={data.name} name={displayName} className="size-24 mb-1" />
        <div>
          <h1 className="text-heading-1 text-foreground">{displayName}</h1>
          <p className="text-body text-muted-foreground font-mono mt-0.5">@{data.name}</p>
          {data.account_number != null && data.account_number <= 1000 && (
            <div className="mt-2">
              <TooltipProvider delayDuration={400}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <EarlyAdopterBadge />
                  </TooltipTrigger>
                  <TooltipContent>One of the first 1,000 users on Astro</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          )}
        </div>
      </div>

      {isOwner && onEditOpen && (
        <Button variant="outline" size="sm" className="w-full" onClick={onEditOpen}>
          Edit profile
        </Button>
      )}

      {data.bio && (
        <p className="text-[13px] text-muted-foreground leading-relaxed">{data.bio}</p>
      )}

      {/* Meta rows */}
      <div className="flex flex-col gap-3">
        <MetaRow icon={<Calendar className="size-3.5" />}>Joined {joinedDate}</MetaRow>
        {data.pronouns && (
          <MetaRow icon={<User2 className="size-3.5" />}>{data.pronouns}</MetaRow>
        )}
        {data.location && (
          <MetaRow icon={<MapPin className="size-3.5" />}>{data.location}</MetaRow>
        )}
        {data.email && (
          <MetaRow icon={<Mail className="size-3.5" />} href={`mailto:${data.email}`}>
            {data.email}
          </MetaRow>
        )}
        {websiteHref && (
          <MetaRow icon={<Globe className="size-3.5" />} href={websiteHref}>
            {websiteLabel}
          </MetaRow>
        )}
      </div>

      {socialLinks.length > 0 && (
        <>
          <div className="h-px bg-border" />
          <div className="flex flex-col gap-3">
            {socialLinks.map(({ icon, label, href }) => (
              <MetaRow key={href} icon={icon} href={href}>
                {label}
              </MetaRow>
            ))}
          </div>
        </>
      )}

      <div className="h-px bg-border" />

      {/* Stats */}
      <div className="grid grid-cols-2 gap-x-4 gap-y-5">
        {stats.map(({ label, value, icon }) => (
          <div key={label}>
            <p className="text-label uppercase text-muted-foreground mb-1">{label}</p>
            <div className="flex items-center gap-1.5">
              <span className="text-muted-foreground">{icon}</span>
              <p className="text-heading-2 text-foreground">{value}</p>
            </div>
          </div>
        ))}
      </div>

      {/* Organizations — only rendered when the account belongs to at least one org */}
      {orgs.length > 0 && (
        <>
          <div className="h-px bg-border" />
          <div className="flex flex-col gap-3">
            <p className="text-label uppercase text-muted-foreground">Organizations</p>
            <div className="flex flex-wrap gap-2">
              {orgs.map((org) => (
                <Link key={org.name} to={`/${org.name}`} title={org.display_name ?? org.name}>
                  <UserAvatar
                    handle={org.name}
                    name={org.display_name ?? org.name}
                    className="size-9 rounded-[6px] transition-opacity hover:opacity-80"
                  />
                </Link>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
