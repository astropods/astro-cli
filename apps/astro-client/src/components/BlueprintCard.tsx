import { useState } from "react";
import { Link } from "react-router";
import { EllipsisVertical, Archive } from "lucide-react";
import { BlueprintIdentity } from "./BlueprintIdentity";
import { UserAvatar } from "./UserAvatar";
import { PrivacyBadge } from "@/components/PrivacyBadge";
import { ArchiveBlueprintDialog } from "@/components/ArchiveBlueprintDialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export interface BlueprintCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  visibility?: string;
  avatarUrl?: string;
  variant?: "default" | "oftenUsedTogether";
  deployCount?: number;
  accountAvatarVersion?: number;
  /** When provided, shows a three-dot menu with an archive option. */
  onArchive?: () => void;
}

export function BlueprintCard({
  slug,
  account,
  name,
  description,
  visibility,
  avatarUrl,
  variant = "default",
  deployCount,
  accountAvatarVersion,
  onArchive,
}: BlueprintCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const formattedDeploys = deployCount != null
    ? new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 }).format(deployCount)
    : "0";
  const deployLabel = deployCount === 1 ? "deploy" : "deploys";

  if (variant === "oftenUsedTogether") {
    return (
      <Link
        to={`/${slug}`}
        className="group flex items-center gap-3 overflow-hidden rounded-md border border-border-strong bg-stone-200 px-3 py-2 transition-all duration-150 hover:bg-stone-300 hover:border-teal-500 hover:shadow-md dark:bg-muted/30 dark:hover:border-teal-400"
      >
        <BlueprintIdentity
          account={account}
          name={name}
          size={36}
          avatarUrl={avatarUrl}
          className="size-9 shrink-0 rounded-sm overflow-hidden"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <h3 className="truncate text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
            {name}
          </h3>
          <p className="flex items-center gap-1.5 font-mono text-mono-sm text-faint-foreground">
            {formattedDeploys} {deployLabel}
            <span className="text-border-strong">•</span>
            {account}
          </p>
        </div>
      </Link>
    );
  }

  return (
    <>
      <Link
        to={`/${slug}`}
        className="group relative flex flex-col overflow-hidden rounded-md border border-stone-400 bg-white transition-all duration-150 hover:bg-stone-25 hover:border-teal-500 hover:shadow-md dark:bg-teal-900/30 dark:hover:border-teal-400"
      >
        {onArchive && (
          <div
            className="absolute top-3 right-3"
            onClick={(e) => { e.preventDefault(); e.stopPropagation(); }}
          >
            <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
                  aria-label="Blueprint options"
                >
                  <EllipsisVertical className="h-4 w-4" />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem
                  onSelect={() => {
                    setMenuOpen(false);
                    setArchiveOpen(true);
                  }}
                >
                  <Archive />
                  Archive <span className="max-w-[120px] truncate font-semibold">{name}</span>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )}
        <div className="flex flex-1 items-start gap-3 p-4 pb-3">
          <BlueprintIdentity
            account={account}
            name={name}
            size={36}
            avatarUrl={avatarUrl}
            className="size-9 shrink-0 rounded-sm overflow-hidden"
          />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <h3 className="flex flex-wrap items-center gap-1.5 text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
              <span className="truncate">{name}</span>
              {visibility === "private" && (
                <PrivacyBadge onClick={(e) => e.preventDefault()} />
              )}
            </h3>
            <p className="line-clamp-3 text-body-sm text-muted-foreground">
              {description}
            </p>
          </div>
        </div>
        <div className="flex items-center justify-between border-t border-border px-4 py-2.5">
          <span className="text-mono-sm font-mono text-faint-foreground">
            {formattedDeploys} {deployLabel}
          </span>
          <span className="flex items-center gap-1.5 text-mono-sm font-mono text-faint-foreground">
            <UserAvatar handle={account} name={account} avatarVersion={accountAvatarVersion} className="!size-4" />
            {account}
          </span>
        </div>
      </Link>

      {onArchive && (
        <ArchiveBlueprintDialog
          open={archiveOpen}
          onOpenChange={setArchiveOpen}
          blueprintName={name}
          account={account}
          onArchived={onArchive}
        />
      )}
    </>
  );
}
