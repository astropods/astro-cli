import { useState, useEffect, useRef } from "react";
import { useBlocker } from "react-router";
import { CheckIcon } from "@heroicons/react/24/outline";
import { Camera, Loader2 } from "lucide-react";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/lib/auth";
import { useUpdateProfile } from "@/api/queries";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import { DeleteAccountDialog } from "@/components/settings/DeleteAccountDialog";
import { AvatarUploadDialog } from "@/components/settings/AvatarUploadDialog";
import { DISPLAY_NAME_MAX_LENGTH } from "@/lib/constants";

const headingClass = {
  h1: "text-heading-1",
  h2: "text-heading-2",
} as const;

function SectionHeader({
  as: Heading = "h2",
  title,
  subtitle,
}: {
  as?: "h1" | "h2";
  title: string;
  subtitle: string;
}) {
  return (
    <div className="space-y-1">
      <Heading className={`${headingClass[Heading]} text-foreground`}>{title}</Heading>
      <p className="text-[13px] text-muted-foreground">{subtitle}</p>
    </div>
  );
}

function SavedIndicator({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <span className="flex items-center gap-1 text-[13px] text-muted-foreground">
      <CheckIcon className="size-3.5" />
      Saved
    </span>
  );
}

function useSavedFlash() {
  const [showSaved, setShowSaved] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => () => clearTimeout(timerRef.current), []);

  const flash = () => {
    setShowSaved(true);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setShowSaved(false), 2000);
  };

  return { showSaved, flash };
}

function ProfileSection() {
  const { user, personalAccount, refresh } = useAuth();
  const updateProfile = useUpdateProfile();
  const initialName = personalAccount?.display_name ?? "";
  const [displayName, setDisplayName] = useState(initialName);
  const [savedName, setSavedName] = useState(initialName);
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false);
  const { showSaved, flash } = useSavedFlash();

  const isDirty = displayName !== savedName;

  const blocker = useBlocker(isDirty);

  useEffect(() => {
    if (blocker.state === "blocked") {
      const leave = window.confirm("You have unsaved changes. Are you sure you want to leave?");
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
    updateProfile.mutate({ display_name: displayName }, {
      onSuccess: () => {
        setSavedName(displayName);
        refresh();
        flash();
      },
    });
  };

  const accountDisplayName = personalAccount?.display_name || personalAccount?.name || "";

  return (
    <div className="flex flex-col gap-5">
      {user && (
        <>
          <div className="flex items-center gap-4">
            <button
              type="button"
              className="group relative cursor-pointer"
              onClick={() => setAvatarDialogOpen(true)}
            >
              <UserAvatar handle={personalAccount?.name ?? user.id} name={accountDisplayName} avatarVersion={personalAccount?.avatar_version} className="size-[72px] text-2xl" />
              <div className="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
                <Camera className="size-5 text-white" />
              </div>
            </button>
            <div>
              <div className="text-sm font-semibold text-foreground">
                {accountDisplayName}
              </div>
              {personalAccount && (
                <div className="font-mono text-xs text-faint-foreground">
                  @{personalAccount.name}
                </div>
              )}
            </div>
          </div>
          {personalAccount && (
            <AvatarUploadDialog
              account={personalAccount.name}
              open={avatarDialogOpen}
              onOpenChange={setAvatarDialogOpen}
              onSuccess={() => { refresh(); flash(); }}
            />
          )}

          <div>
            <Label size="md">Display name</Label>
            <Input
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              maxLength={DISPLAY_NAME_MAX_LENGTH}
            />
          </div>

          <div className="flex items-center gap-2">
            <Button
              disabled={!isDirty || updateProfile.isPending}
              onClick={handleSave}
              className="self-start"
            >
              {updateProfile.isPending && (
                <Loader2 size={14} className="spinner-delayed" />
              )}
              Save changes
            </Button>
            <SavedIndicator visible={showSaved} />
          </div>
        </>
      )}
    </div>
  );
}

function AccountSection() {
  const { user, personalAccount, refresh } = useAuth();
  const [open, setOpen] = useState(false);
  const { showSaved, flash } = useSavedFlash();

  const handleSuccess = () => {
    refresh();
    setOpen(false);
    flash();
  };

  return (
    <div className="flex flex-col gap-5">
      {user && (
        <div>
          <Label size="md">Email</Label>
          <Input defaultValue={user.email} disabled />
        </div>
      )}
      <div>
        <Label size="md">Username</Label>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[13px] text-foreground">
            @{personalAccount?.name}
          </span>
          {personalAccount && (
            <ChangeUsernameDialog
              currentName={personalAccount.name}
              open={open}
              onOpenChange={setOpen}
              onSuccess={handleSuccess}
            />
          )}
          <SavedIndicator visible={showSaved} />
        </div>
      </div>
    </div>
  );
}

function DangerZone() {
  const [open, setOpen] = useState(false);

  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-destructive/30 bg-destructive/5 px-5 py-4">
      <div>
        <div className="text-[13px] font-semibold text-foreground">Delete account</div>
        <p className="text-[12px] text-muted-foreground">
          Permanently delete your account and all associated data. This cannot
          be undone.
        </p>
      </div>
      <Button
        variant="outline"
        className="shrink-0 border-destructive/30 bg-surface text-destructive hover:bg-destructive/[0.08] hover:text-destructive active:bg-destructive/15 active:text-destructive"
        onClick={() => setOpen(true)}
      >
        Delete account
      </Button>
      <DeleteAccountDialog open={open} onOpenChange={setOpen} />
    </div>
  );
}

export default function AccountSettings() {
  return (
    <>
      <SectionHeader title="Profile" subtitle="Your public identity on Astro" />
      <ProfileSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Account" subtitle="Email, password, and authentication" />
      <AccountSection />
      <hr className="my-2 border-border" />
      <SectionHeader title="Danger Zone" subtitle="These actions are irreversible" />
      <DangerZone />
    </>
  );
}
