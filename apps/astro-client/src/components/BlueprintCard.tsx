import { useState } from "react";
import { Link } from "react-router";
import { EllipsisHorizontalIcon, ArchiveBoxIcon, ArrowRightIcon } from "@heroicons/react/24/outline";
import { BlueprintIdentity } from "./BlueprintIdentity";
import { UserAvatar } from "./UserAvatar";
import { PrivacyBadge } from "@/components/PrivacyBadge";
import { InlineBadge } from "@/components/InlineBadge";
import { ArchiveBlueprintDialog } from "@/components/ArchiveBlueprintDialog";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

const compactFormatter = new Intl.NumberFormat("en-US", {
  notation: "compact",
  maximumFractionDigits: 1,
});

export interface BlueprintCardProps {
  slug: string;
  account: string;
  name: string;
  description: string;
  visibility?: string;
  avatarUrl?: string;
  variant?: "default" | "oftenUsedTogether" | "list";
  deployCount?: number;
  heartCount?: number;
  isDraft?: boolean;
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
  heartCount,
  isDraft = false,
  onArchive,
}: BlueprintCardProps) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [archiveOpen, setArchiveOpen] = useState(false);
  const formattedDeploys = deployCount != null ? compactFormatter.format(deployCount) : "0";
  const deployLabel = deployCount === 1 ? "deploy" : "deploys";

  if (variant === "oftenUsedTogether") {
    return (
      <Link
        to={`/${slug}`}
        className="group flex items-center gap-3 overflow-hidden rounded-md border border-border-strong bg-stone-100 px-3 py-2 transition-all duration-150 hover:bg-stone-200 hover:border-teal-500 hover:shadow-md dark:bg-muted/30 dark:hover:border-teal-400"
      >
        <BlueprintIdentity
          account={account}
          name={name}
          size={36}
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

  if (variant === "list") {
    const formattedHearts = heartCount != null ? compactFormatter.format(heartCount) : "0";
    return (
      <>
        <div className="flex items-center gap-4 rounded-lg border border-border bg-background px-4 py-3">
          <Link to={`/${slug}`} className="flex min-w-0 flex-1 items-center gap-3">
            <BlueprintIdentity
              account={account}
              name={name}
              size={36}
              url={avatarUrl}
              className="size-9 shrink-0 overflow-hidden rounded-sm"
            />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate text-heading-4 text-foreground">{name}</span>
                {isDraft
                  ? <InlineBadge shape="pill" variant="soft" className="normal-case" style={{ color: "var(--color-yellow-700)", background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)" }}>Finish setup</InlineBadge>
                  : visibility === "private" && <PrivacyBadge />
                }
              </div>
              {description && (
                <p className="truncate text-body-sm text-muted-foreground">{description}</p>
              )}
            </div>
          </Link>
          <div className="flex shrink-0 items-center gap-2">
            {isDraft ? (
              <Button asChild size="sm" variant="outline">
                <Link to={`/${slug}`}>
                  Continue setup
                  <ArrowRightIcon className="size-3.5" />
                </Link>
              </Button>
            ) : (
              <>
                <span className="font-mono text-mono-sm text-muted-foreground">
                  {formattedDeploys} deploys · {formattedHearts} hearts
                </span>
                <Button asChild size="sm">
                  <Link to={`/deploy/${slug}`}>Deploy</Link>
                </Button>
                <Button asChild size="sm" variant="outline">
                  <Link to={`/${slug}`}>View →</Link>
                </Button>
              </>
            )}
            {onArchive && (
              <div onClick={(e) => { e.preventDefault(); e.stopPropagation(); }}>
                <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
                  <DropdownMenuTrigger asChild>
                    <button
                      type="button"
                      className="flex h-7 w-7 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                      aria-label="Blueprint options"
                    >
                      <EllipsisHorizontalIcon className="h-4 w-4" />
                    </button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="rounded-[10px] p-0">
                    <DropdownMenuItem
                      variant="destructive"
                      onSelect={() => { setMenuOpen(false); setArchiveOpen(true); }}
                      className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
                    >
                      <ArchiveBoxIcon className="h-4 w-4" />
                      Archive agent
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )}
          </div>
        </div>
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

  const cardHref = `/${slug}`;

  return (
    <>
      <Link
        to={cardHref}
        className={cn(
          "group relative flex flex-col overflow-hidden rounded-md border transition-all duration-150 hover:bg-stone-25 hover:border-teal-500 hover:shadow-md dark:hover:border-teal-400",
          isDraft ? "border-dashed border-stone-400 bg-transparent" : "border-stone-400 bg-white dark:bg-teal-900/30"
        )}
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
                  className="flex h-7 w-7 items-center justify-center rounded-md text-foreground transition-colors hover:bg-accent"
                  aria-label="Blueprint options"
                >
                  <EllipsisHorizontalIcon className="h-4 w-4" />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="rounded-[10px] p-0">
                <DropdownMenuItem
                  variant="destructive"
                  onSelect={() => {
                    setMenuOpen(false);
                    setArchiveOpen(true);
                  }}
                  className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
                >
                  <ArchiveBoxIcon className="h-4 w-4" />
                  Archive agent
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
            className="size-9 shrink-0 rounded-sm overflow-hidden"
          />
          <div className="flex min-w-0 flex-1 flex-col gap-1 pr-1">
            <h3 className="flex flex-wrap items-center gap-1.5 text-heading-4 text-foreground transition-colors group-hover:text-teal-500 dark:group-hover:text-teal-400">
              <span className="truncate">{name}</span>
              {isDraft
                ? <InlineBadge shape="pill" variant="soft" className="normal-case" style={{ color: "var(--color-yellow-700)", background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)" }}>Finish setup</InlineBadge>
                : visibility === "private" && <PrivacyBadge onClick={(e) => e.preventDefault()} />
              }
            </h3>
            <p className="line-clamp-3 text-body-sm text-muted-foreground">
              {description}
            </p>
          </div>
        </div>
        <div className={cn("flex items-center justify-between border-t px-4 py-2.5", isDraft ? "border-dashed border-border" : "border-border")}>
          <span className="text-mono-sm font-mono text-faint-foreground">
            {formattedDeploys} {deployLabel}
          </span>
          <span className="flex items-center gap-1.5 text-mono-sm font-mono text-faint-foreground">
            <UserAvatar handle={account} name={account} className="!size-4" />
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
