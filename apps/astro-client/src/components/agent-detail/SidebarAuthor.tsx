import { Link } from "react-router";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { SidebarSection } from "./SidebarSection";
import { UserAvatar } from "@/components/UserAvatar";
import type { AgentCardAuthor } from "@/lib/api";

const AVATAR_THRESHOLD = 3;

export interface SidebarAuthorProps {
  /** Agent card authors — falls back to account owner when empty. */
  authors: AgentCardAuthor[];
  /** Account owner name (fallback when no agent card authors). */
  ownerName: string;
  /** Account handle (fallback when no agent card authors). */
  ownerHandle: string;
  /** Account ID (used to seed the preset avatar). */
  ownerId?: string;
  /** Account owner profile picture URL. */
  ownerProfilePictureUrl?: string;
}

function AuthorFullCard({ author }: { author: AgentCardAuthor }) {
  const inner = (
    <div className="flex items-center gap-3">
      <UserAvatar accountId={author.account ?? author.name} name={author.name} className="h-9 w-9" />
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
  ownerId,
  ownerProfilePictureUrl,
}: SidebarAuthorProps) {
  // Fall back to account owner when no agent card authors
  const hasCardAuthors = authors.length > 0;
  const compact = hasCardAuthors && authors.length > AVATAR_THRESHOLD;

  return (
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
                        <UserAvatar accountId={author.account ?? author.name} name={author.name} className="h-8 w-8" />
                      </Link>
                    ) : (
                      <div>
                        <UserAvatar accountId={author.account ?? author.name} name={author.name} className="h-8 w-8" />
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
          {ownerId && <UserAvatar accountId={ownerId} name={ownerName} profilePictureUrl={ownerProfilePictureUrl} className="h-9 w-9" />}
          <div className="flex flex-col">
            <span className="text-[13px] font-medium text-foreground">{ownerName}</span>
            <span className="text-[11px] text-[var(--faint-foreground)] font-mono">
              @{ownerHandle}
            </span>
          </div>
        </div>
      )}
    </SidebarSection>
  );
}
