import { useState, useEffect, useCallback } from "react";
import { useBlocker } from "react-router";
import { UserIcon } from "@heroicons/react/24/outline";
import { ProtectedRoute } from "@/components/ProtectedRoute";
import { UserAvatar } from "@/components/UserAvatar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth, getUserDisplayName } from "@/lib/auth";
import { useUpdateProfile } from "@/api/queries";
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

function splitDisplayName(name: string): { first_name: string; last_name: string } {
  const trimmed = name.trim();
  const spaceIndex = trimmed.indexOf(" ");
  if (spaceIndex === -1) return { first_name: trimmed, last_name: "" };
  return {
    first_name: trimmed.slice(0, spaceIndex),
    last_name: trimmed.slice(spaceIndex + 1),
  };
}

function ProfileSection({
  isDirty,
  onDirtyChange,
}: {
  isDirty: boolean;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const { user, personalAccount } = useAuth();
  const updateProfile = useUpdateProfile();
  const initialName = user ? getUserDisplayName(user) : "";
  const [displayName, setDisplayName] = useState(initialName);

  const dirty = displayName !== initialName;

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);

  const handleSave = () => {
    updateProfile.mutate(splitDisplayName(displayName));
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
              <label className="mb-1.5 block font-mono text-mono-md uppercase tracking-widest text-ink-muted">
                Display name
              </label>
              <Input
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
          </div>

          <Button
            disabled={!isDirty || updateProfile.isPending}
            onClick={handleSave}
            className="self-start"
          >
            {updateProfile.isPending ? "Saving…" : "Save changes"}
          </Button>
        </>
      )}
    </div>
  );
}

function AccountSection() {
  const { user, personalAccount } = useAuth();

  return (
    <div className="flex flex-col gap-5">
      {user && (
        <div>
          <label className="mb-1.5 block font-mono text-mono-md uppercase tracking-widest text-ink-muted">
            Email
          </label>
          <Input defaultValue={user.email} disabled />
        </div>
      )}
      <div>
        <label className="mb-1.5 block font-mono text-mono-md uppercase tracking-widest text-ink-muted">
          Username
        </label>
        <div className="flex items-baseline gap-2">
          <span className="font-mono text-[13px] text-ink">
            @{personalAccount?.name}
          </span>
          <Button variant="link" className="h-auto p-0 text-[13px]">
            Change username
          </Button>
        </div>
      </div>
    </div>
  );
}

function SettingsContent() {
  const [isDirty, setIsDirty] = useState(false);
  const handleDirtyChange = useCallback((dirty: boolean) => setIsDirty(dirty), []);

  // Block in-app navigation when there are unsaved changes
  const blocker = useBlocker(isDirty);

  // Handle browser/tab close with beforeunload
  useEffect(() => {
    if (!isDirty) return;
    const handler = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty]);

  // If the user confirms they want to leave, proceed
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
          <ProfileSection isDirty={isDirty} onDirtyChange={handleDirtyChange} />
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
