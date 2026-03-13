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
    <div className="pt-5 mt-5 border-t border-border-strong">
      <SidebarSection title="Authors">
        {hasCardAuthors ? (
          compact ? (
            // >3 authors: avatar-only row with tooltips
            <TooltipProvider>
              <div className="flex flex-wrap gap-2">
                {authors.map((author, i) => (
                  <Tooltip key={`${author.name}-${i}`}>
                    <TooltipTrigger asChild>
                      {author.account ? (
                        <Link to={`/${author.account}`} className="hover:opacity-80 transition-opacity">
                          <AuthorAvatar name={author.name} className="h-8 w-8 text-xs" />
                        </Link>
                      ) : (
                        <div>
                          <AuthorAvatar name={author.name} className="h-8 w-8 text-xs" />
                        </div>
                      )}
                    </TooltipTrigger>
                    <TooltipContent side="top" sideOffset={4}>
                      {author.name}{author.account ? ` (@${author.account})` : ""}
                    </TooltipContent>
                  </Tooltip>
                ))}
              </div>
            </TooltipProvider>
          ) : (
            // ≤3 authors: full cards
            <div className="flex flex-col gap-3">
              {authors.map((author, i) => (
                <AuthorFullCard key={`${author.name}-${i}`} author={author} />
              ))}
            </div>
          )
        ) : (
          // Fallback: account owner
          <div className="flex items-center gap-3">
            {ownerProfilePictureUrl ? (
              <img
                src={ownerProfilePictureUrl}
                alt={ownerName}
                className="h-9 w-9 shrink-0 rounded-full object-cover"
              />
            ) : (
              <AuthorAvatar name={ownerName} />
            )}
            <div className="flex flex-col">
              <span className="text-[13px] font-medium text-foreground">{ownerName}</span>
              <span className="text-[11px] text-[var(--faint-foreground)] font-mono">
                @{ownerHandle}
              </span>
            </div>
          </div>
        )}
      </SidebarSection>
    </div>
  );
}
