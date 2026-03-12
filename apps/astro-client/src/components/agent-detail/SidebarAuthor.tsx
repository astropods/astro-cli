import { Link } from "react-router";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { SidebarSection } from "./SidebarSection";
import type { AgentCardAuthor } from "@/lib/api";

const AVATAR_THRESHOLD = 3;

export interface SidebarAuthorProps {
  /** Agent card authors — falls back to account owner when empty. */
  authors: AgentCardAuthor[];
  /** Account owner name (fallback when no agent card authors). */
  ownerName: string;
  /** Account handle (fallback when no agent card authors). */
  ownerHandle: string;
  /** Account owner profile picture URL. */
  ownerProfilePictureUrl?: string;
}

function AuthorAvatar({
  name,
  className = "h-9 w-9",
}: {
  name: string;
  className?: string;
}) {
  const initial = name.charAt(0).toUpperCase();
  return (
    <div
      className={`flex items-center justify-center rounded-full bg-stone-300 text-sm font-semibold text-muted-foreground dark:bg-teal-900 shrink-0 ${className}`}
    >
      {initial}
    </div>
  );
}

function AuthorFullCard({ author }: { author: AgentCardAuthor }) {
  const inner = (
    <div className="flex items-center gap-3">
      <AuthorAvatar name={author.name} />
      <div className="flex flex-col min-w-0">
        <span className="text-[13px] font-medium text-foreground truncate">
          {author.name}
        </span>
        {author.account && (
          <span className="text-[11px] text-[var(--faint-foreground)] font-mono truncate">
            @{author.account}
          </span>
        )}
      </div>
    </div>
  );

  if (author.account) {
    return (
      <Link to={`/${author.account}`} className="hover:opacity-80 transition-opacity">
        {inner}
      </Link>
    );
  }
  return inner;
}

export function SidebarAuthor({
  authors,
  ownerName,
  ownerHandle,
  ownerProfilePictureUrl,
}: SidebarAuthorProps) {
  // Fall back to account owner when no agent card authors
  const hasCardAuthors = authors.length > 0;
  const compact = hasCardAuthors && authors.length > AVATAR_THRESHOLD;

  return (
    <SidebarSection title="Creator">
      <div className="flex items-center gap-3.5">
        {profilePictureUrl ? (
          <img
            src={profilePictureUrl}
            alt={name}
            className="h-9 w-9 shrink-0 rounded-full object-cover"
          />
        ) : (
          <div className="flex h-9 w-9 items-center justify-center rounded-full bg-stone-300 text-sm font-semibold text-muted-foreground dark:bg-teal-900">
            {initial}
          </div>
        )}
        <div className="flex flex-col">
          <span className="text-[15px] leading-5 font-semibold text-foreground">{name}</span>
          <span className="text-[12px] leading-4 font-mono text-muted-foreground">
            @{handle}
          </span>
        </div>
      </div>
    </SidebarSection>
  );
}
