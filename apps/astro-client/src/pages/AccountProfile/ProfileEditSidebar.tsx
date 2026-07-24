import { useState } from "react";
import { getApiErrorMessage, type AccountPublic } from "@/lib/api";
import {
  useUpdateProfile,
  useUpdateAccountDisplayName,
  useUpdateAccountProfile,
  useUploadAvatar,
} from "@/api/queries/accounts";
import { UserAvatar } from "@/components/UserAvatar";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Input, inputBase, inputFocusWithin } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { stripProtocol, withProtocol } from "@/lib/website";
import { Textarea } from "@/components/ui/textarea";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { SocialLinksEditor } from "@/components/account-profile/SocialLinksEditor";
import { PronounsSelect } from "@/components/account-profile/PronounsSelect";
import { bustAvatar } from "@/lib/avatar-bust";
import { getDisplayNameError } from "@/lib/account-display-name";
import { SaveButton } from "@/components/ui/save-button";
import { Camera, X } from "lucide-react";

interface ProfileEditSidebarProps {
  data: AccountPublic;
  onClose: () => void;
  variant?: "personal" | "org";
}

export function ProfileEditSidebar({ data, onClose, variant = "personal" }: ProfileEditSidebarProps) {
  const isOrg = variant === "org";

  const updateProfile = useUpdateProfile(data.name);
  const updateDisplayName = useUpdateAccountDisplayName();
  const updateAccountProfile = useUpdateAccountProfile();
  const uploadAvatar = useUploadAvatar();
  const { patchAccount, refreshUserData } = useAuth();
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);

  const [displayName, setDisplayName] = useState(data.display_name ?? "");
  const [bio, setBio] = useState(data.bio ?? "");
  const [location, setLocation] = useState(data.location ?? "");
  const [pronouns, setPronouns] = useState(data.pronouns ?? "");
  const [website, setWebsite] = useState(withProtocol(data.website ?? ""));
  const [socialLinks, setSocialLinks] = useState<[string, string, string, string]>(() => {
    const links = data.social_links ?? [];
    return [links[0] ?? "", links[1] ?? "", links[2] ?? "", links[3] ?? ""];
  });

  const displayNameKind = isOrg ? "organization" : "personal";
  const trimmedDisplayName = displayName.trim();
  const displayNameError = getDisplayNameError(
    displayName,
    displayNameKind,
  );
  const [savePending, setSavePending] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const isSaving =
    savePending ||
    (isOrg ? updateDisplayName.isPending : updateProfile.isPending) ||
    updateAccountProfile.isPending;

  async function handleSave() {
    setSaveError(null);
    if (displayNameError || isSaving) return;
    setSavePending(true);
    try {
      const saveDisplayName = isOrg
        ? updateDisplayName
            .mutateAsync({
              account: data.name,
              displayName: trimmedDisplayName,
            })
            .then(() => {
              patchAccount(data.name, { display_name: trimmedDisplayName });
            })
        : updateProfile.mutateAsync({ display_name: trimmedDisplayName });

      await Promise.all([
        saveDisplayName,
        updateAccountProfile.mutateAsync({
          account: data.name,
          bio,
          location,
          website,
          social_links: socialLinks.filter((s) => s.trim() !== ""),
          ...(!isOrg && { pronouns, local_timezone: "" }),
        }),
      ]);
      if (!isOrg) {
        patchAccount(data.name, { display_name: trimmedDisplayName });
      }
      await refreshUserData();
      onClose();
    } catch (error: unknown) {
      setSaveError(getApiErrorMessage(error, "Failed to save. Please try again."));
    } finally {
      setSavePending(false);
    }
  }

  return (
    <div className="relative z-10 flex flex-col gap-5 px-5 py-6 sm:px-6 md:py-7 md:h-full md:overflow-y-auto">
      <div className="flex items-center justify-between">
        <h2 className="text-heading-4 text-foreground">{isOrg ? "Edit org profile" : "Edit profile"}</h2>
        <Button variant="ghost" size="icon-sm" onClick={onClose} className="text-muted-foreground">
          <X className="size-4" />
        </Button>
      </div>

      <button
        type="button"
        className="group relative self-start cursor-pointer"
        onClick={() => setAvatarDialogOpen(true)}
      >
        <UserAvatar handle={data.name} name={displayName || data.name} avatarUrl={data.avatar_url} className="size-16" />
        <div className="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
          <Camera className="size-4 text-white" />
        </div>
      </button>

      <AvatarUploadDialog
        open={avatarDialogOpen}
        onOpenChange={setAvatarDialogOpen}
        onUpload={async (file) => {
          await uploadAvatar.mutateAsync({ account: data.name, file });
        }}
        isPending={uploadAvatar.isPending}
        title={isOrg ? "Upload org image" : "Upload profile image"}
        onSuccess={(blob) => {
          bustAvatar(data.name, blob);
          void refreshUserData();
        }}
      />

      <div className="flex flex-col gap-4">
        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Name</p>
          <Input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder={isOrg ? "Organization name" : "Display name"}
            aria-invalid={!!displayNameError || undefined}
            aria-describedby={
              displayNameError ? "profile-display-name-error" : undefined
            }
            className={cn(
              "h-8 text-body-sm",
              displayNameError &&
                "border-destructive focus-visible:ring-destructive/20",
            )}
          />
          {displayNameError && (
            <p id="profile-display-name-error" className="mt-1 text-[11px] text-destructive">
              {displayNameError}
            </p>
          )}
        </div>

        <div>
          <div className="flex items-baseline justify-between mb-1">
            <p className="text-body-sm text-muted-foreground">Bio</p>
            <span className="text-[11px] text-faint-foreground">{bio.length}/160</span>
          </div>
          <Textarea
            value={bio}
            onChange={(e) => setBio(e.target.value)}
            maxLength={160}
            placeholder={isOrg ? "Tell people about your organization" : "Tell people a bit about yourself"}
            rows={3}
            className="resize-none text-body-sm"
          />
        </div>

        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Location</p>
          <Input
            value={location}
            onChange={(e) => setLocation(e.target.value)}
            placeholder="City, Country"
            className="h-8 text-body-sm"
          />
        </div>

        {!isOrg && (
          <div>
            <p className="text-body-sm text-muted-foreground mb-1">Pronouns</p>
            <PronounsSelect value={pronouns} onValueChange={setPronouns} className="h-8 text-body-sm" />
          </div>
        )}

        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Website</p>
          <div className={cn(inputBase, inputFocusWithin, "flex items-center px-0")}>
            <span className="shrink-0 pl-3 pr-2 text-muted-foreground text-body-sm select-none">https://</span>
            <span className="shrink-0 select-none text-border">|</span>
            <Input
              value={stripProtocol(website)}
              onChange={(e) => setWebsite(withProtocol(e.target.value))}
              placeholder="yoursite.com"
              className="border-0 bg-transparent shadow-none h-8 rounded-none focus-visible:ring-0 pl-2.5 pr-3 text-body-sm"
            />
          </div>
        </div>

        <div className="h-px bg-border" />
        <p className="text-body-sm font-medium text-foreground -mb-1">Social accounts</p>
        <SocialLinksEditor links={socialLinks} onChange={setSocialLinks} compact />
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex gap-2">
          <SaveButton
            onClick={handleSave}
            isSaving={isSaving}
          />
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
        </div>
        {saveError && <p className="text-[11px] text-destructive">{saveError}</p>}
      </div>
    </div>
  );
}
