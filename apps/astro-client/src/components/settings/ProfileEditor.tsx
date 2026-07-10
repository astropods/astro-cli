import { useState, useEffect } from "react";
import { useBlocker } from "react-router";
import { Camera } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import { SaveButton } from "@/components/ui/save-button";
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
import { getApiErrorMessage } from "@/lib/api";
import { bustAvatar } from "@/lib/avatar-bust";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import {
  getDisplayNameError,
  type AccountDisplayNameKind,
} from "@/lib/account-display-name";

const PERMISSION_TOOLTIP = "You need admin or owner access to edit this";
const SAVE_ERROR_FALLBACK = "Failed to save. Please try again.";

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
  /** Account kind used for display-name validation */
  displayNameKind?: AccountDisplayNameKind;
}

export function ProfileEditor({
  accountName,
  currentDisplayName,
  avatarDialogTitle,
  currentAvatarUrl,
  onSave,
  isSaving,
  readOnly,
  displayNameKind = "personal",
}: ProfileEditorProps) {
  const { refreshUserData } = useAuth();
  const uploadAvatar = useUploadAvatar();
  const [displayName, setDisplayName] = useState(currentDisplayName);
  const [savedName, setSavedName] = useState(currentDisplayName);
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const [savePending, setSavePending] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // Sync when upstream data changes (e.g. after session refresh)
  useEffect(() => {
    setDisplayName(currentDisplayName);
    setSavedName(currentDisplayName);
  }, [currentDisplayName]);

  const isDirty = displayName !== savedName;
  const trimmedDisplayName = displayName.trim();
  const displayNameClientError = getDisplayNameError(
    displayName,
    displayNameKind,
  );
  const displayNameError = displayNameClientError ?? saveError;
  const saving = isSaving || savePending;

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
    if (displayNameClientError || saving) return;
    setSaveError(null);
    setSavePending(true);
    onSave(trimmedDisplayName)
      .then(() => {
        setDisplayName(trimmedDisplayName);
        setSavedName(trimmedDisplayName);
      })
      .catch((error: unknown) => {
        setSaveError(getApiErrorMessage(error, SAVE_ERROR_FALLBACK));
      })
      .finally(() => {
        setSavePending(false);
      });
  };

  const resolvedDisplayName = currentDisplayName || accountName;

  const permissionTip = readOnly ? (
    <TooltipContent>{PERMISSION_TOOLTIP}</TooltipContent>
  ) : null;

  return (
    <TooltipProvider delayDuration={300}>
      <div className="flex flex-col gap-5">
        <div className="flex min-w-0 items-center gap-4">
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
          <div className="min-w-0 flex-1">
            <div
              className="min-w-0 max-w-full truncate text-sm font-semibold text-foreground"
              title={resolvedDisplayName}
            >
              {resolvedDisplayName}
            </div>
            <div
              className="min-w-0 max-w-full truncate font-mono text-xs text-faint-foreground"
              title={`@${accountName}`}
            >
              {`@${accountName}`}
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
                    onChange={(e) => {
                      setDisplayName(e.target.value);
                      setSaveError(null);
                    }}
                    placeholder="Add a display name"
                    disabled={readOnly}
                    aria-invalid={!!displayNameError || undefined}
                    aria-describedby={
                      displayNameError ? "display-name-error" : undefined
                    }
                  />
                </div>
              </TooltipTrigger>
              {permissionTip}
            </Tooltip>
            {isDirty && (
              <SaveButton
                className="shrink-0"
                isSaving={saving}
                onClick={handleSave}
              />
            )}
          </div>
          <p
            id="display-name-error"
            aria-hidden={!displayNameError}
            aria-live="polite"
            className={`mt-1.5 min-h-4 max-w-sm text-xs ${
              displayNameError ? "text-destructive" : "invisible"
            }`}
          >
            {displayNameError}
          </p>
        </div>
      </div>
    </TooltipProvider>
  );
}
