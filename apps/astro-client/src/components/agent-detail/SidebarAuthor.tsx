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

  const primaryAuthor = resolvedAuthors[0];
  const secondaryAuthors = resolvedAuthors.slice(1);
  const name = primaryAuthor?.name ?? ownerName;
  const handle = primaryAuthor?.account ?? ownerHandle;
  const profilePictureUrl = ownerProfilePictureUrl;
  const initial = name.charAt(0).toUpperCase();

  return (
    <SidebarSection title="Authors">
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
      {secondaryAuthors.length > 0 && (
        <div className="mt-2.5 space-y-1">
          {secondaryAuthors.map((author) => (
            <p key={`${author.name}-${author.account ?? "unknown"}`} className="text-[12px] leading-4 font-mono text-muted-foreground">
              {author.name}
              {author.account ? ` (@${author.account})` : ""}
            </p>
          ))}
        </div>
      )}
    </SidebarSection>
  );
}
