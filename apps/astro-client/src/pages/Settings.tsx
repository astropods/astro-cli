import { useState, useEffect, useRef } from "react";
import { useBlocker } from "react-router";
import { UserIcon, CheckIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth, getUserDisplayName } from "@/lib/auth";
import { useUpdateProfile } from "@/api/queries";
import { ChangeUsernameDialog } from "@/components/settings/ChangeUsernameDialog";
import {
  SidebarLayout,
  SidebarNav,
  SidebarNavItem,
  SidebarBody,
} from "@/components/ui/sidebar-layout";

function SectionHeader({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <div className="space-y-1">
      <h2 className="text-heading-2 font-bold text-ink">{title}</h2>
      <p className="text-[13px] text-ink-muted">{subtitle}</p>
    </div>
  );
}

function SavedIndicator({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <span className="flex items-center gap-1 text-[13px] text-ink-muted">
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

function splitDisplayName(name: string): { first_name: string; last_name: string } {
  const trimmed = name.trim();
  const spaceIndex = trimmed.indexOf(" ");
  if (spaceIndex === -1) return { first_name: trimmed, last_name: "" };
  return {
    first_name: trimmed.slice(0, spaceIndex),
    last_name: trimmed.slice(spaceIndex + 1),
  };
}

function ProfileSection() {
  const { user, personalAccount, refresh } = useAuth();
  const updateProfile = useUpdateProfile();
  const initialName = user ? getUserDisplayName(user) : "";
  const [displayName, setDisplayName] = useState(initialName);
  const [savedName, setSavedName] = useState(initialName);
  const { showSaved, flash } = useSavedFlash();

  const isDirty = displayName !== savedName;

  // Block in-app navigation when there are unsaved changes
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

  // Handle browser/tab close with beforeunload
  useEffect(() => {
    if (!isDirty) return;
    const handler = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty]);

  const handleSave = () => {
    updateProfile.mutate(splitDisplayName(displayName), {
      onSuccess: () => {
        setSavedName(displayName);
        refresh();
        flash();
      },
    });
  };

  return (
    <div className="flex flex-col gap-5">
      {user && (
        <>
          <div className="flex items-center gap-4">
            <UserAvatar user={user} className="size-[72px] text-2xl" />
            <div>
              <div className="text-sm font-semibold text-ink">
                {getUserDisplayName(user)}
              </div>
              {personalAccount && (
                <div className="font-mono text-xs text-ink-faint">
                  @{personalAccount.name}
                </div>
              )}
            </div>
          </div>

          <div className="flex flex-col gap-5">
            <div>
              <Label size="md">Display name</Label>
              <Input
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
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
          <span className="font-mono text-[13px] text-ink">
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

function SettingsContent() {
  return (
    <div className="@container w-full flex-1 overflow-y-auto bg-surface px-6 pb-6 pt-8 md:px-8 md:pb-8 md:pt-10 max-w-[820px] mx-auto">
      <SidebarLayout>
        <SidebarNav label="Settings">
          <SidebarNavItem active>
            <span className="flex items-center gap-2">
              <UserIcon className="size-3.5" />
              Profile
            </span>
          </SidebarNavItem>
        </SidebarNav>
        <SidebarBody>
          <SectionHeader title="Profile" subtitle="Your public identity on Astro" />
          <ProfileSection />
          <hr className="my-2 border-border" />
          <SectionHeader title="Account" subtitle="Email, password, and authentication" />
          <AccountSection />
        </SidebarBody>
      </SidebarLayout>
    </div>
  );
}

export default function Settings() {
  return (
    <ProtectedRoute>
      <SettingsContent />
    </ProtectedRoute>
  );
}
