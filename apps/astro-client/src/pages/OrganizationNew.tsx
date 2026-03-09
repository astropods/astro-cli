import { useState, useCallback, useMemo } from "react";
import { useNavigate } from "react-router";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { useCreateAccount } from "../api/queries/accounts";
import { useAuth } from "../lib/auth";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";
import { AccountNameInput } from "@/components/AccountNameInput";
import { useAccountNameValidation } from "@/hooks/use-account-name";
import type { ApiError } from "@/lib/api";

function OrganizationNewContent() {
  const [name, setName] = useState("");
  const [invites, setInvites] = useState<InviteEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { checkAuth, accounts } = useAuth();
  const createAccount = useCreateAccount();
  const excludeFromInvite = useMemo(
    () => new Set(accounts.filter((a) => a.type === "personal").map((a) => a.name)),
    [accounts]
  );

  const { isChecking, isAvailable, displayError } = useAccountNameValidation(name, 4);

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

  const handleChange = useCallback(
    (value: string) => {
      setName(value);
      setError(null);
    },
    []
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
            <Label htmlFor="org-name" className="mb-1.5 block">
              Organization username
            </Label>
            <AccountNameInput
              value={name}
              onChange={handleChange}
              placeholder="my-org"
              autoFocus
              isChecking={isChecking}
              isAvailable={isAvailable}
              displayError={displayError}
            />
          </div>

          <div>
            <Label className="mb-1.5 block">
              Invite members
            </Label>
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
