import { useState, useEffect } from "react";
import { useBlocker } from "react-router";
import { Camera, Loader2 } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useUploadAvatar } from "@/api/queries";
import { useAuth } from "@/lib/auth";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { bustAvatar } from "@/lib/avatar-bust";
import { SavedIndicator } from "@/components/settings/SettingsShared";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { DISPLAY_NAME_MAX_LENGTH } from "@/lib/constants";

const PERMISSION_TOOLTIP = "You need admin or owner access to edit this";

interface ProfileEditorProps {
  /** Account slug used for avatar upload */
  accountName: string;
  /** Current display name from the account */
  currentDisplayName: string;
  /** Title for the avatar upload dialog */
  avatarDialogTitle: string;
  /** Current versioned avatar URL from the server */
  currentAvatarUrl?: string;
  /** Called with the new display name; should return a promise that resolves on success */
  onSave: (displayName: string) => Promise<void>;
  /** Whether the save mutation is in flight */
  isSaving: boolean;
  /** When true, all editing controls are disabled */
  readOnly?: boolean;
}

export function ProfileEditor({
  accountName,
  currentDisplayName,
  avatarDialogTitle,
  currentAvatarUrl,
  onSave,
  isSaving,
  readOnly,
}: ProfileEditorProps) {
  const { refreshUserData } = useAuth();
  const uploadAvatar = useUploadAvatar();
  const [displayName, setDisplayName] = useState(currentDisplayName);
  const [savedName, setSavedName] = useState(currentDisplayName);
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const { showSaved, flash } = useSavedFlash();

  // Sync when upstream data changes (e.g. after session refresh)
  useEffect(() => {
    setDisplayName(currentDisplayName);
    setSavedName(currentDisplayName);
  }, [currentDisplayName]);

  const isDirty = displayName !== savedName;

  const blocker = useBlocker(isDirty);

  useEffect(() => {
    if (blocker.state === "blocked") {
      const leave = window.confirm(
        "You have unsaved changes. Are you sure you want to leave?",
      );
      if (leave) {
        blocker.proceed();
      } else {
        blocker.reset();
      }
    }
  }, [blocker]);

  useEffect(() => {
    if (!isDirty) return;
    const handler = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty]);

  const handleSave = () => {
    const trimmed = displayName.trim();
    onSave(trimmed).then(() => {
      setDisplayName(trimmed);
      setSavedName(trimmed);
      flash();
    });
  };

  const resolvedDisplayName = currentDisplayName || accountName;

  const permissionTip = readOnly ? (
    <TooltipContent>{PERMISSION_TOOLTIP}</TooltipContent>
  ) : null;

  return (
    <TooltipProvider delayDuration={300}>
      <div className="flex flex-col gap-5">
        <div className="flex items-center gap-4">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                className="group relative cursor-pointer disabled:cursor-not-allowed"
                onClick={() => setAvatarDialogOpen(true)}
                disabled={readOnly}
              >
                <UserAvatar
                  handle={accountName}
                  name={resolvedDisplayName}
                  avatarUrl={currentAvatarUrl}
                  className="size-[72px] text-2xl"
                />
                {!readOnly && (
                  <div className="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                    <Camera className="size-5 text-white" />
                  </div>
                )}
              </button>
            </TooltipTrigger>
            {permissionTip}
          </Tooltip>
          <div>
            <div className="text-sm font-semibold text-foreground">
              {resolvedDisplayName}
            </div>
            <div className="font-mono text-xs text-faint-foreground">
              @{accountName}
            </div>
          </div>
        </div>
        <AvatarUploadDialog
          open={avatarDialogOpen}
          onOpenChange={setAvatarDialogOpen}
          onUpload={async (file) => {
            await uploadAvatar.mutateAsync({ account: accountName, file });
          }}
          isPending={uploadAvatar.isPending}
          title={avatarDialogTitle}
          onSuccess={(blob) => {
            bustAvatar(accountName, blob);
            void refreshUserData();
            flash();
          }}
        />

        <div>
          <Label size="md">Display name</Label>
          <div className="flex flex-wrap items-center gap-3">
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="min-w-[200px] max-w-sm flex-1">
                  <Input
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    maxLength={DISPLAY_NAME_MAX_LENGTH}
                    placeholder="Add a display name"
                    disabled={readOnly}
                  />
                </div>
              </TooltipTrigger>
              {permissionTip}
            </Tooltip>
            {isDirty && (
              <Button
                className="shrink-0"
                disabled={isSaving}
                onClick={handleSave}
              >
                {isSaving && <Loader2 size={14} className="spinner-delayed" />}
                Save changes
              </Button>
            )}
            <SavedIndicator visible={showSaved} />
          </div>
        </div>
      </div>
    </TooltipProvider>
  );
}
