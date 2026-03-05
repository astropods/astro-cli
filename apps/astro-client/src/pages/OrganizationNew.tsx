import { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { useNavigate } from "react-router";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { useCreateAccount, useCheckAccountName } from "../api/queries/accounts";
import { useAuth } from "../lib/auth";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";
import type { ApiError } from "@/lib/api";

function validateName(name: string): string | null {
  if (name.length < 4) return "Must be at least 4 characters";
  if (name.length > 39) return "Must be at most 39 characters";
  if (!/^[a-z]/.test(name)) return "Must start with a letter";
  if (name.endsWith("-")) return "Must not end with a hyphen";
  if (/--/.test(name)) return "Must not contain consecutive hyphens";
  if (!/^[a-z0-9-]+$/.test(name))
    return "Only lowercase letters, numbers, and hyphens";
  return null;
}

function OrganizationNewContent() {
  const [name, setName] = useState("");
  const [debouncedName, setDebouncedName] = useState("");
  const [invites, setInvites] = useState<InviteEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { checkAuth, accounts } = useAuth();
  const createAccount = useCreateAccount();
  const excludeFromInvite = useMemo(
    () => new Set(accounts.filter((a) => a.type === "personal").map((a) => a.name)),
    [accounts]
  );
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const clientError = name.length > 0 ? validateName(name) : null;
  const shouldCheck = name.length >= 4 && !clientError;

  useEffect(() => {
    timerRef.current = setTimeout(
      () => setDebouncedName(shouldCheck ? name : ""),
      shouldCheck ? 300 : 0,
    );
    return () => clearTimeout(timerRef.current);
  }, [name, shouldCheck]);

  const nameCheck = useCheckAccountName(debouncedName);
  const isChecking = shouldCheck && (debouncedName !== name || nameCheck.isFetching);
  const serverAvailable =
    nameCheck.data?.available === true && debouncedName === name;
  const serverReason =
    nameCheck.data?.available === false && debouncedName === name
      ? (nameCheck.data as { reason?: string }).reason || "Already taken"
      : null;

  const isAvailable = shouldCheck && !isChecking && serverAvailable;
  const displayError = clientError || serverReason;

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError(null);
      if (!isAvailable) return;

      try {
        const invitations = invites
          .filter((inv) => inv.valid)
          .map((inv) => ({ value: inv.value, kind: inv.kind, role: "member" as const }));
        await createAccount.mutateAsync({
          name,
          type: "organization",
          ...(invitations.length > 0 && { invitations }),
        });
      } catch (err: unknown) {
        const apiErr = err as ApiError;
        setError(
          apiErr.error_description ||
            apiErr.error ||
            "Failed to create organization"
        );
        return;
      }

      try {
        await checkAuth();
      } catch {
        // ignore — org was created successfully
      }
      navigate(`/${name}`);
    },
    [name, invites, isAvailable, createAccount, checkAuth, navigate]
  );

  return (
    <div className="flex flex-1 items-start justify-center p-6 md:p-8">
      <div className="w-full max-w-md pt-12">
        <h1 className="text-2xl font-bold">Create an organization</h1>
        <p className="text-muted-foreground mt-1 text-pretty text-sm">
          Organizations let you collaborate with others and manage agents as a
          team.
        </p>

        <form onSubmit={handleSubmit} className="mt-6 space-y-3">
          <div>
            <label
              htmlFor="org-name"
              className="mb-1.5 block text-sm font-medium"
            >
              Organization username
            </label>
            <div className="relative">
              <Input
                id="org-name"
                type="text"
                value={name}
                onChange={(e) => {
                  setName(
                    e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "")
                  );
                  setError(null);
                }}
                placeholder="my-org"
                autoFocus
                maxLength={39}
                aria-invalid={!!displayError || undefined}
                className="pr-9"
              />
              <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm">
                {name.length === 0
                  ? ""
                  : isChecking
                    ? "\u2026"
                    : isAvailable
                      ? "\u2713"
                      : displayError
                        ? "\u2717"
                        : ""}
              </span>
            </div>
            <div className="mt-1 min-h-5 text-xs">
              {name.length > 0 && displayError && (
                <p className="text-destructive text-pretty">{displayError}</p>
              )}
              {isChecking && (
                <p className="text-muted-foreground">
                  Checking availability...
                </p>
              )}
              {isAvailable && <p className="text-green-600">Available</p>}
            </div>
          </div>

          <div>
            <label className="mb-1.5 block text-sm font-medium">
              Invite members
            </label>
            <InviteInput entries={invites} onChange={setInvites} exclude={excludeFromInvite} />
            <p className="text-muted-foreground mt-1 text-xs">
              Invitations will be sent after the organization is created.
            </p>
          </div>

          {error && <p className="text-destructive text-pretty text-sm">{error}</p>}

          <Button
            type="submit"
            size="lg"
            disabled={createAccount.isPending || !isAvailable || invites.some((e) => !e.valid)}
            className="mt-6 w-full"
          >
            {createAccount.isPending
              ? "Creating..."
              : "Create organization"}
          </Button>
        </form>
      </div>
    </div>
  );
}

export default function OrganizationNew() {
  return (
    <ProtectedRoute>
      <OrganizationNewContent />
    </ProtectedRoute>
  );
}
