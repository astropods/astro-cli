import { useState } from "react";
import type { MetaFunction } from "react-router";
import { Link } from "react-router";
import { Camera, Loader2, Pencil, Trash2 } from "lucide-react";
import { SocialLinksEditor } from "@/components/account-profile/SocialLinksEditor";
import { PronounsSelect } from "@/components/account-profile/PronounsSelect";
import { useAuth } from "@/lib/auth";
import { useUpdateProfile, useUploadAvatar } from "@/api/queries";
import { useAccount, useUpdateAccountProfile, useResetAvatar } from "@/api/queries/accounts";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useSavedFlash } from "@/hooks/use-saved-flash";
import { SectionHeader, SavedIndicator } from "@/components/settings/SettingsShared";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { UserAvatar } from "@/components/UserAvatar";
import { Input, inputBase, inputFocusWithin } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { DISPLAY_NAME_MAX_LENGTH } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { stripProtocol, withProtocol } from "@/lib/website";
import type { AccountPublic } from "@/lib/api";

export const meta: MetaFunction = () => [{ title: "Profile - Settings | Astro" }];

// ── Helpers ────────────────────────────────────────────────────────────────

function toSocialLinks(links?: string[]): [string, string, string, string] {
  const l = links ?? [];
  return [l[0] ?? "", l[1] ?? "", l[2] ?? "", l[3] ?? ""];
}

// ── Sub-components ─────────────────────────────────────────────────────────


interface AvatarSectionProps {
  accountName: string;
  displayName: string;
  onSuccess: () => void;
}

function AvatarSection({ accountName, displayName, onSuccess }: AvatarSectionProps) {
  const uploadAvatar = useUploadAvatar();
  const resetAvatar = useResetAvatar();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);

  const handleReset = async () => {
    await resetAvatar.mutateAsync(accountName);
    onSuccess();
  };

  return (
    <div className="flex flex-col items-center pt-1">
      <div className="relative">
        <Button
          variant="ghost"
          className="p-0 h-auto rounded-full"
          onClick={() => setDropdownOpen(true)}
        >
          <UserAvatar handle={accountName} name={displayName} className="size-[200px] text-5xl" />
        </Button>

        <DropdownMenu open={dropdownOpen} onOpenChange={setDropdownOpen}>
          <DropdownMenuTrigger asChild>
            <Button
              variant="outline"
              size="sm"
              className="absolute bottom-0 left-1/2 -translate-x-1/2 translate-y-1/2 gap-1.5 shadow-md bg-card border-border"
            >
              <Pencil className="size-3" />
              Edit
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="center" side="bottom" sideOffset={8}>
            <DropdownMenuItem onClick={() => setDialogOpen(true)}>
              <Camera className="size-3.5" />
              Change photo
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={handleReset}
              disabled={resetAvatar.isPending}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="size-3.5 text-destructive" />
              Remove photo
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <AvatarUploadDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onUpload={async (file) => { await uploadAvatar.mutateAsync({ account: accountName, file }); }}
        isPending={uploadAvatar.isPending}
        title="Upload profile image"
        onSuccess={onSuccess}
      />
    </div>
  );
}


// ── Form ───────────────────────────────────────────────────────────────────

interface ProfileFormProps {
  account: AccountPublic;
  personalAccountName: string;
  refresh: () => void;
}

function ProfileForm({ account, personalAccountName, refresh }: ProfileFormProps) {
  const updateProfile = useUpdateProfile(personalAccountName);
  const updateAccountProfile = useUpdateAccountProfile();
  const { showSaved, flash } = useSavedFlash();

  const [displayName, setDisplayName] = useState(account.display_name ?? "");
  const [bio, setBio] = useState(account.bio ?? "");
  const [location, setLocation] = useState(account.location ?? "");
  const [email, setEmail] = useState(account.email ?? "");
  const [pronouns, setPronouns] = useState(account.pronouns ?? "");
  const [website, setWebsite] = useState(withProtocol(account.website ?? ""));
  const [socialLinks, setSocialLinks] = useState(() => toSocialLinks(account.social_links));

  const isSaving = updateProfile.isPending || updateAccountProfile.isPending;
  const displayNameEmpty = displayName.trim() === "";

  const isDirty =
    displayName !== (account.display_name ?? "") ||
    bio !== (account.bio ?? "") ||
    location !== (account.location ?? "") ||
    email !== (account.email ?? "") ||
    pronouns !== (account.pronouns ?? "") ||
    website !== withProtocol(account.website ?? "") ||
    socialLinks.some((v, i) => v !== ((account.social_links ?? [])[i] ?? ""));

  const handleSave = async () => {
    await updateProfile.mutateAsync({ display_name: displayName });
    await updateAccountProfile.mutateAsync({
      account: personalAccountName,
      bio,
      location,
      email,
      pronouns,
      website,
      social_links: socialLinks.filter(s => s.trim() !== ""),
    });
    refresh();
    flash();
  };

  return (
    <div className="flex items-start justify-between gap-10">
      <div className="flex flex-col gap-6 min-w-0 flex-1">
      <div className="max-w-sm">
        <Label size="md">Display name</Label>
        <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} maxLength={DISPLAY_NAME_MAX_LENGTH} aria-invalid={displayNameEmpty} />
        {displayNameEmpty && (
          <p className="mt-1 text-sm text-destructive">Display name can't be empty</p>
        )}
      </div>

      <div className="max-w-sm">
        <div className="flex items-baseline justify-between">
          <Label size="md">Bio</Label>
          <span className="text-[11px] text-faint-foreground">{bio.length}/160</span>
        </div>
        <Textarea
          value={bio}
          onChange={(e) => setBio(e.target.value)}
          maxLength={160}
          placeholder="Tell people a bit about yourself"
          rows={3}
          className="resize-none"
        />
      </div>

      <div className="max-w-sm">
        <Label size="md">Location</Label>
        <Input value={location} onChange={(e) => setLocation(e.target.value)} placeholder="City, Country" />
      </div>

      <div className="max-w-sm">
        <Label size="md">Email</Label>
        <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" type="email" />
      </div>

      <div className="max-w-sm">
        <Label size="md">Pronouns</Label>
        <PronounsSelect value={pronouns} onValueChange={setPronouns} />
      </div>

      <div className="max-w-sm">
        <Label size="md">Website</Label>
        <div className={cn("flex items-center px-0", inputBase, inputFocusWithin)}>
          <span className="shrink-0 pl-3.5 pr-2 text-muted-foreground text-body select-none">https://</span>
          <span className="shrink-0 select-none text-border">|</span>
          <Input
            value={stripProtocol(website)}
            onChange={(e) => setWebsite(withProtocol(e.target.value))}
            placeholder="yoursite.com"
            className="border-0 bg-transparent shadow-none h-auto rounded-none focus-visible:ring-0 pl-2.5 pr-3.5 py-2"
          />
        </div>
      </div>

      <hr className="border-border" />

      <div className="flex flex-col gap-1.5">
        <p className="text-sm font-semibold text-foreground">Social accounts</p>
        <p className="text-xs text-muted-foreground mb-3">
          Paste a profile URL or use <span className="font-mono">@username</span> to link to an Astro account.
          Platform icons are detected automatically.
        </p>
        <div className="max-w-sm">
          <SocialLinksEditor links={socialLinks} onChange={setSocialLinks} />
        </div>
      </div>

      <div className="flex items-center gap-3">
        <Button onClick={handleSave} disabled={isSaving || !isDirty || displayNameEmpty}>
          {isSaving && <Loader2 size={14} className="spinner-delayed" />}
          Save changes
        </Button>
        <SavedIndicator visible={showSaved} />
      </div>
      </div>

      <AvatarSection
        accountName={personalAccountName}
        displayName={displayName || personalAccountName}
        onSuccess={() => { refresh(); flash(); }}
      />
    </div>
  );
}

// ── Page ───────────────────────────────────────────────────────────────────

export default function ProfileSettings() {
  const { personalAccount, refreshUserData } = useAuth();
  const { data: account } = useAccount(personalAccount?.name ?? "");

  if (!personalAccount || !account) return null;

  return (
    <>
      <div className="flex items-start justify-between gap-4 pb-6 border-b border-border">
        <SectionHeader title="Profile" subtitle="Your public identity on Astro" className="border-none pb-0" />
        <Button variant="outline" size="sm" asChild className="shrink-0 mt-0.5">
          <Link to={`/${personalAccount.name}`}>View profile</Link>
        </Button>
      </div>
      <ProfileForm key={personalAccount.name} account={account} personalAccountName={personalAccount.name} refresh={refreshUserData} />
    </>
  );
}
