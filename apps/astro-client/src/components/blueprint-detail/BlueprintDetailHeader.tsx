import { useState } from "react";
import { Camera } from "lucide-react";
import { InlineBadge } from "@/components/InlineBadge";
import { BlueprintIdentity } from "@/components/BlueprintIdentity";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { useUploadBlueprintAvatar } from "@/api/queries";
import { bustAgentAvatar } from "@/lib/avatar-bust";
import { cn } from "@/lib/utils";

export interface BlueprintDetailHeaderProps {
  account: string;
  name: string;
  categories: string[];
  canEdit?: boolean;
  isDraft?: boolean;
}

export function BlueprintDetailHeader({
  account,
  name,
  categories,
  canEdit = false,
  isDraft = false,
}: BlueprintDetailHeaderProps) {
  const hasCategories = categories.length > 0;
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
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
      <div className={cn("flex gap-4", hasCategories ? "items-start" : "items-center")}>
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
        <div className="min-w-0">
          <h1 className="flex flex-wrap items-center gap-2 font-mono text-xl font-bold text-foreground">
            {name}
            {isDraft && (
              <InlineBadge shape="pill" variant="soft" className="normal-case" style={{ color: "var(--color-yellow-700)", background: "color-mix(in oklch, var(--color-yellow-700) 12%, transparent)" }}>
                Set up
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
    </header>
  );
}
