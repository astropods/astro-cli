import { Link } from "react-router";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { SidebarSection } from "./SidebarSection";
import type { AgentCardAuthor } from "@/lib/api";

export interface SidebarAuthorProps {
  authors: AgentCardAuthor[];
  ownerName: string;
  ownerHandle: string;
  ownerProfilePictureUrl?: string;
}

export function SidebarAuthor({
  authors,
  ownerName,
  ownerHandle,
  ownerProfilePictureUrl,
}: SidebarAuthorProps) {
  const validAuthors = authors.filter((author) => author.name.trim().length > 0);
  const resolvedAuthors = validAuthors.length > 0
    ? validAuthors
    : [{ name: ownerName, account: ownerHandle }];

  return (
    <SidebarSection title="Authors">
      <div className="space-y-3.5">
        {resolvedAuthors.map((author, idx) => {
          const displayName = author.name;
          const displayHandle = author.account ?? ownerHandle;
          const initial = displayName.charAt(0).toUpperCase();
          const profilePictureUrl = idx === 0 ? ownerProfilePictureUrl : undefined;

          return (
            <div key={`${displayName}-${displayHandle}-${idx}`} className="flex items-center gap-3.5">
              {profilePictureUrl ? (
                <img
                  src={profilePictureUrl}
                  alt={displayName}
                  className="h-9 w-9 shrink-0 rounded-full object-cover"
                />
              ) : (
                <div className="flex h-9 w-9 items-center justify-center rounded-full bg-stone-300 text-sm font-semibold text-muted-foreground dark:bg-teal-900">
                  {initial}
                </div>
              )}
              <div className="flex flex-col">
                <span className="text-[15px] leading-5 font-semibold text-foreground">{displayName}</span>
                <span className="text-[12px] leading-4 font-mono text-muted-foreground">
                  @{displayHandle}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </SidebarSection>
  );
}
