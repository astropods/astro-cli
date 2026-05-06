import { useState } from "react";
import type { AccountPublic } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import {
  useUpdateProfile,
  useUpdateAccountProfile,
  useUploadAvatar,
} from "@/api/queries/accounts";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Input, inputBase, inputFocusWithin } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { stripProtocol, withProtocol } from "@/lib/website";
import { Textarea } from "@/components/ui/textarea";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { SocialLinksEditor } from "@/components/account-profile/SocialLinksEditor";
import { PronounsSelect } from "@/components/account-profile/PronounsSelect";
import { DISPLAY_NAME_MAX_LENGTH } from "@/lib/constants";
import { Camera, Loader2, X } from "lucide-react";

interface ProfileEditSidebarProps {
  data: AccountPublic;
  onClose: () => void;
}

export function ProfileEditSidebar({ data, onClose }: ProfileEditSidebarProps) {
  const { refresh } = useAuth();
  const updateProfile = useUpdateProfile(data.name);
  const updateAccountProfile = useUpdateAccountProfile();
  const uploadAvatar = useUploadAvatar();
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);

  // Initialized directly from props — no useEffect sync needed
  const [displayName, setDisplayName] = useState(data.display_name ?? "");
  const [bio, setBio] = useState(data.bio ?? "");
  const [location, setLocation] = useState(data.location ?? "");
  const [email, setEmail] = useState(data.email ?? "");
  const [pronouns, setPronouns] = useState(data.pronouns ?? "");
  const [website, setWebsite] = useState(withProtocol(data.website ?? ""));
  const [socialLinks, setSocialLinks] = useState<[string, string, string, string]>(() => {
    const links = data.social_links ?? [];
    return [links[0] ?? "", links[1] ?? "", links[2] ?? "", links[3] ?? ""];
  });

  const isSaving = updateProfile.isPending || updateAccountProfile.isPending;

  async function handleSave() {
    if (displayName.trim()) {
      await updateProfile.mutateAsync({ display_name: displayName });
    }
    await updateAccountProfile.mutateAsync({
      account: data.name,
      bio,
      location,
      email,
      pronouns,
      website,
      social_links: socialLinks.filter((s) => s.trim() !== ""),
      local_timezone: "",
    });
    refresh();
    onClose();
  }

  return (
    <div className="relative z-10 flex flex-col gap-5 px-6 py-7 h-full overflow-y-auto">
      <div className="flex items-center justify-between">
        <h2 className="text-heading-4 text-foreground">Edit profile</h2>
        <Button variant="ghost" size="icon-sm" onClick={onClose} className="text-muted-foreground">
          <X className="size-4" />
        </Button>
      </div>

      <button
        type="button"
        className="group relative self-start cursor-pointer"
        onClick={() => setAvatarDialogOpen(true)}
      >
        <UserAvatar handle={data.name} name={displayName || data.name} className="size-16" />
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
        title="Upload profile image"
        onSuccess={() => refresh()}
      />

      <div className="flex flex-col gap-4">
        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Name</p>
          <Input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            maxLength={DISPLAY_NAME_MAX_LENGTH}
            placeholder="Display name"
            className="h-8 text-body-sm"
          />
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
            placeholder="Tell people a bit about yourself"
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
        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Email</p>
          <Input
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            type="email"
            className="h-8 text-body-sm"
          />
        </div>
        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Pronouns</p>
          <PronounsSelect value={pronouns} onValueChange={setPronouns} className="h-8 text-body-sm" />
        </div>
        <div>
          <p className="text-body-sm text-muted-foreground mb-1">Website</p>
          <div className={cn("flex items-center px-0", inputBase, inputFocusWithin)}>
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

      <div className="flex gap-2">
        <Button size="sm" onClick={handleSave} disabled={isSaving}>
          {isSaving && <Loader2 className="size-3.5 animate-spin" />}
          Save
        </Button>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
