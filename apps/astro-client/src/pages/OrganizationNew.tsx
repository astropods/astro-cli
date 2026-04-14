import { useState, useCallback, useMemo } from "react";
import { useNavigate, type MetaFunction } from "react-router";
import { ProtectedRoute } from "../components/ProtectedRoute";
import { useCreateAccount } from "../api/queries/accounts";
import { useAuth } from "../lib/auth";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";
import { AccountNameInput } from "@/components/AccountNameInput";
import { useAccountNameValidation } from "@/hooks/use-account-name";
import type { ApiError } from "@/lib/api";
import { DISPLAY_NAME_MAX_LENGTH } from "@/lib/constants";

export const meta: MetaFunction = () => [{ title: "New Organization | Astro" }];

function OrganizationNewContent() {
  const [name, setName] = useState("");
  const [displayName, setDisplayName] = useState("");
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
          display_name: displayName.trim(),
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
    [name, displayName, invites, isAvailable, createAccount, checkAuth, navigate]
  );

  const handleChange = useCallback(
    (value: string) => {
      setName(value);
      setError(null);
    },
    []
  );

  return (
    <div className="mx-auto max-w-[480px] px-6 pt-20">
      <h1 className="text-heading-1 mb-2">Create an organization</h1>
      <p className="text-muted-foreground mb-8 leading-relaxed">
        Organizations let you collaborate with others and manage agents as a
        team.
      </p>

      <form onSubmit={handleSubmit}>
        <div className="mb-4">
          <Label size="md">Organization name</Label>
          <Input
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            placeholder="My Organization"
            autoFocus
            maxLength={DISPLAY_NAME_MAX_LENGTH}
          />
        </div>

        <div className="mb-4">
          <Label size="md">Organization username</Label>
          <AccountNameInput
            value={name}
            onChange={handleChange}
            placeholder="my-org"
            isChecking={isChecking}
            isAvailable={isAvailable}
            displayError={displayError}
          />
        </div>

        <div className="mb-6">
          <Label size="md">Invite members</Label>
          <InviteInput entries={invites} onChange={setInvites} exclude={excludeFromInvite} />
          <p className="text-muted-foreground mt-1 text-xs">
            Invitations will be sent after the organization is created.
          </p>
        </div>

        {error && <p className="text-destructive mb-4 text-sm">{error}</p>}

        <Button
          type="submit"
          size="lg"
          disabled={createAccount.isPending || !isAvailable || invites.some((e) => !e.valid)}
          className="w-full"
        >
          {createAccount.isPending
            ? "Creating..."
            : "Create organization"}
        </Button>
      </form>
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
