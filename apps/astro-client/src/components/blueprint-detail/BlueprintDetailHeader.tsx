import { useState } from "react";
import { Camera } from "lucide-react";
import { ArchiveBoxIcon, EllipsisHorizontalIcon } from "@heroicons/react/24/outline";
import { InlineBadge } from "@/components/InlineBadge";
import { StatusBadge } from "@/components/StatusBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { ArchiveBlueprintDialog } from "@/components/ArchiveBlueprintDialog";
import { useUploadBlueprintAvatar } from "@/api/queries";
import { bustAgentAvatar } from "@/lib/avatar-bust";
import { cn } from "@/lib/utils";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export interface BlueprintDetailHeaderProps {
  account: string;
  name: string;
  categories: string[];
  canEdit?: boolean;
  isDraft?: boolean;
  onArchive?: () => void;
  /** Server-emitted versioned avatar URL; falls back to BlueprintIdentity's chain when absent. */
  avatarUrl?: string;
}

export function BlueprintDetailHeader({
  account,
  name,
  categories,
  canEdit = false,
  isDraft = false,
  onArchive,
  avatarUrl,
}: BlueprintDetailHeaderProps) {
  const hasCategories = categories.length > 0;
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const [archiveDialogOpen, setArchiveDialogOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const uploadAvatar = useUploadBlueprintAvatar();

  const avatarImage = (
    <BlueprintIdentity
      account={account}
      name={name}
      size={56}
      url={avatarUrl}
      className="size-14 shrink-0 rounded-sm overflow-hidden border border-stone-200 dark:border-border"
    />
  );

  return (
    <header className="mb-6 border-b border-border-strong pb-6">
      {onArchive && (
        <ArchiveBlueprintDialog
          open={archiveDialogOpen}
          onOpenChange={setArchiveDialogOpen}
          blueprintName={name}
          account={account}
          onArchived={onArchive}
        />
      )}
      <div className={cn("flex min-w-0 gap-4", hasCategories ? "items-start" : "items-center")}>
        {canEdit ? (
          <>
            <button
              type="button"
              className="group relative shrink-0 cursor-pointer"
              onClick={() => setAvatarDialogOpen(true)}
            >
              {avatarImage}
              <div className="absolute inset-0 flex items-center justify-center rounded-sm bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                <Camera className="size-5 text-white" />
              </div>
            </button>
            <AvatarUploadDialog
              open={avatarDialogOpen}
              onOpenChange={setAvatarDialogOpen}
              onUpload={async (file) => {
                await uploadAvatar.mutateAsync({ account, name, file });
                bustAgentAvatar(account, name, file);
              }}
              isPending={uploadAvatar.isPending}
              title="Upload blueprint image"
              cropShape="rect"
            />
          </>
        ) : (
          avatarImage
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1">
            <h1 className="m-0 flex flex-wrap items-center gap-2 font-mono text-xl font-bold leading-none text-foreground">
              {name}
              {isDraft && (
                <StatusBadge color="warning">Finish setup</StatusBadge>
              )}
            </h1>
            {onArchive && (
              <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    className="flex h-7 w-7 items-center justify-center rounded-sm text-foreground transition-colors hover:bg-accent"
                    aria-label="Blueprint options"
                  >
                    <EllipsisHorizontalIcon className="h-4 w-4" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="rounded-[10px] p-0">
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setMenuOpen(false);
                      setArchiveDialogOpen(true);
                    }}
                    className="gap-[10px] rounded-none px-[14px] py-[10px] text-[length:var(--text-heading-4)]"
                  >
                    <ArchiveBoxIcon className="h-4 w-4" />
                    Archive
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
          {hasCategories && (
            <div className="mt-2 flex flex-wrap gap-2">
              {categories.map((tag) => (
                <InlineBadge
                  key={tag}
                  className="rounded-[4px] bg-surface text-muted-foreground dark:bg-surface dark:text-muted-foreground"
                >
                  {tag}
                </InlineBadge>
              ))}
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
