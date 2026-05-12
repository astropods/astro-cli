import type { ReactNode } from "react";
import { Link } from "react-router";
import type { AccountPublic } from "@/lib/api";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { TooltipProvider } from "@/components/ui/tooltip";
import { detectSocialLink, type SocialLinkDisplay } from "@/lib/social-links";
import { Calendar, Globe, Mail, MapPin, User2 } from "lucide-react";

export interface StatItem {
  label: string;
  value: number;
  icon: ReactNode;
}

interface ProfileSidebarShellProps {
  data: AccountPublic;
  isAdmin: boolean;
  onEditOpen?: () => void;
  dateLabel: string;
  stats: StatItem[];
  badge?: ReactNode;
  pronouns?: string;
  email?: string;
  children?: ReactNode;
}

function MetaRow({
  icon,
  children,
  href,
}: {
  icon: ReactNode;
  children: ReactNode;
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
    return (
      <Link to={href} className={`${base} transition-colors hover:text-foreground`}>
        {content}
      </Link>
    );
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

function StatCell({ label, value, icon }: StatItem) {
  return (
    <div>
      <p className="text-label uppercase text-muted-foreground mb-1">{label}</p>
      <div className="flex items-center gap-1.5">
        <span className="text-muted-foreground">{icon}</span>
        <p className="text-heading-2 text-foreground">{value}</p>
      </div>
    </div>
  );
}

export function ProfileSidebarShell({
  data,
  isAdmin,
  onEditOpen,
  dateLabel,
  stats,
  badge,
  pronouns,
  email,
  children,
}: ProfileSidebarShellProps) {
  const displayName = data.display_name || data.name;
  const date = new Date(data.created_at).toLocaleDateString("en-US", {
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

  return (
    <TooltipProvider delayDuration={400}>
      <div className="relative z-10 flex flex-col gap-6 px-6 py-7 h-full overflow-y-auto overflow-x-hidden">
        <div className="flex flex-col gap-2">
          <UserAvatar handle={data.name} name={displayName} className="size-24 mb-1" />
          <div>
            <h1 className="text-heading-1 text-foreground break-words hyphens-auto">{displayName}</h1>
            <p className="text-body text-muted-foreground font-mono mt-0.5">@{data.name}</p>
            {badge && <div className="mt-2">{badge}</div>}
          </div>
        </div>

        {isAdmin && onEditOpen && (
          <Button variant="outline" size="sm" className="w-full" onClick={onEditOpen}>
            Edit profile
          </Button>
        )}

        {data.bio && (
          <p className="text-[13px] text-muted-foreground leading-relaxed">{data.bio}</p>
        )}

        <div className="flex flex-col gap-3">
          <MetaRow icon={<Calendar className="size-3.5" />}>{dateLabel} {date}</MetaRow>
          {pronouns && <MetaRow icon={<User2 className="size-3.5" />}>{pronouns}</MetaRow>}
          {data.location && <MetaRow icon={<MapPin className="size-3.5" />}>{data.location}</MetaRow>}
          {email && (
            <MetaRow icon={<Mail className="size-3.5" />} href={`mailto:${email}`}>{email}</MetaRow>
          )}
          {websiteHref && (
            <MetaRow icon={<Globe className="size-3.5" />} href={websiteHref}>{websiteLabel}</MetaRow>
          )}
        </div>

        {socialLinks.length > 0 && (
          <>
            <div className="h-px bg-border" />
            <div className="flex flex-col gap-3">
              {socialLinks.map(({ icon, label, href }) => (
                <MetaRow key={href} icon={icon} href={href}>{label}</MetaRow>
              ))}
            </div>
          </>
        )}

        <div className="h-px bg-border" />

        <div className="grid grid-cols-2 gap-x-4 gap-y-5">
          {stats.map(({ label, value, icon }) => (
            <StatCell key={label} label={label} value={value} icon={icon} />
          ))}
        </div>

        {children}
      </div>
    </TooltipProvider>
  );
}
