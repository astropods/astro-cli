import { useState } from "react";
import { Camera } from "lucide-react";
import { ArchiveBoxIcon } from "@heroicons/react/24/outline";
import { InlineBadge } from "@/components/InlineBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { ArchiveBlueprintDialog } from "@/components/ArchiveBlueprintDialog";
import { Button } from "@/components/ui/button";
import { useUploadBlueprintAvatar } from "@/api/queries";
import { bustAgentAvatar } from "@/lib/avatar-bust";
import { cn } from "@/lib/utils";

export interface BlueprintDetailHeaderProps {
  account: string;
  name: string;
  categories: string[];
  canEdit?: boolean;
  isDraft?: boolean;
  onArchive?: () => void;
}

export function BlueprintDetailHeader({
  account,
  name,
  categories,
  canEdit = false,
  isDraft = false,
  onArchive,
}: BlueprintDetailHeaderProps) {
  const hasCategories = categories.length > 0;
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const [archiveDialogOpen, setArchiveDialogOpen] = useState(false);
  const uploadAvatar = useUploadBlueprintAvatar();

  const avatarImage = (
    <BlueprintIdentity
      account={account}
      name={name}
      size={56}
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
      <div className="flex flex-col gap-3 sm:flex-row sm:gap-4 sm:items-start">
        <div className={cn("flex min-w-0 flex-1 gap-4", hasCategories ? "items-start" : "items-center")}>
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
            <h1 className="flex flex-wrap items-center gap-2 font-mono text-xl font-bold text-foreground">
              {name}
              {isDraft && (
                <InlineBadge shape="pill" variant="soft" className="normal-case" style={{ color: "var(--color-yellow-700)", background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)" }}>
                  Finish setup
                </InlineBadge>
              )}
            </h1>
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
        {onArchive && (
          <Button variant="outline" size="sm" className="shrink-0 self-start sm:self-center text-[var(--color-coral-600)] border-[var(--color-coral-300)] hover:text-[var(--color-coral-600)] dark:border-[var(--color-coral-800)]" onClick={() => setArchiveDialogOpen(true)}>
            <ArchiveBoxIcon className="size-4" />
            Archive
          </Button>
        )}
      </div>
    </header>
  );
}
