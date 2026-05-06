import { Globe } from "lucide-react";
import { AstroIcon } from "@/components/ui/astro-icon";
import { GitHubIcon } from "@/components/ui/svgs/githubIcon";
import { LinkedInIcon } from "@/components/ui/svgs/linkedinIcon";
import { XIcon } from "@/components/ui/svgs/xIcon";

export type SocialLinkDisplay = { icon: React.ReactNode; label: string; href: string };

export function detectSocialLink(value: string): SocialLinkDisplay | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (trimmed.startsWith('@')) {
    const handle = trimmed.slice(1);
    return { icon: <AstroIcon className="size-3.5" />, label: trimmed, href: `/${handle}` };
  }
  const normalized = /^https?:\/\//.test(trimmed) ? trimmed : `https://${trimmed}`;
  try {
    const url = new URL(normalized);
    const host = url.hostname.replace(/^www\./, '');
    if (host === 'github.com') {
      const user = url.pathname.slice(1).split('/')[0];
      return { icon: <GitHubIcon className="size-3.5" />, label: user || host, href: normalized };
    }
    if (host === 'linkedin.com') {
      const label = url.pathname.replace(/^\/in\//, '').replace(/\/$/, '') || host;
      return { icon: <LinkedInIcon className="size-3.5" />, label, href: normalized };
    }
    if (host === 'x.com' || host === 'twitter.com') {
      const user = url.pathname.slice(1).split('/')[0];
      return { icon: <XIcon className="size-3.5" />, label: user ? `@${user}` : host, href: normalized };
    }
    return { icon: <Globe className="size-3.5" />, label: url.hostname, href: normalized };
  } catch {
    return { icon: <Globe className="size-3.5" />, label: trimmed, href: normalized };
  }
}
