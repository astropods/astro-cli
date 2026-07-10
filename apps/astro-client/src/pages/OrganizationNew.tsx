import { useState, useCallback, useMemo } from "react";
import { useNavigate, type MetaFunction } from "react-router";
import { useCreateAccount } from "../api/queries/accounts";
import { useAuth } from "../lib/auth";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { InviteInput, type InviteEntry } from "@/components/InviteInput";
import { AccountNameInput } from "@/components/AccountNameInput";
import { useAccountNameValidation, validateAccountName } from "@/hooks/use-account-name";
import { getApiErrorMessage } from "@/lib/api";
import { getDisplayNameValidationError } from "@/lib/account-display-name";

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
  const displayNameError = getDisplayNameValidationError(displayName, "organization");

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError(null);
      const trimmedDisplayName = displayName.trim();
      if (!trimmedDisplayName) {
        setError("Organization name is required");
        return;
      }
      if (displayNameError) return;
      if (invites.some((entry) => !entry.valid)) {
        setError("Remove invalid invitations before creating the organization");
        return;
      }
      const nameError = !name ? "Organization username is required" : validateAccountName(name, 4);
      if (nameError) {
        setError(nameError);
        return;
      }
      if (isChecking) {
        setError("Checking username availability, try again in a moment");
        return;
      }
      if (!isAvailable) {
        setError(displayError || "Choose an available organization username");
        return;
      }

      try {
        const invitations = invites
          .filter((inv) => inv.valid)
          .map((inv) => ({ value: inv.value, kind: inv.kind, role: "member" as const }));
        await createAccount.mutateAsync({
          name,
          type: "organization",
          display_name: trimmedDisplayName,
          ...(invitations.length > 0 && { invitations }),
        });
      } catch (err: unknown) {
        setError(getApiErrorMessage(err, "Failed to create organization"));
        return;
      }

      try {
        await checkAuth();
      } catch {
        // ignore — org was created successfully
      }
      navigate(`/${name}`);
    },
    [name, displayName, displayNameError, invites, isChecking, isAvailable, displayError, createAccount, checkAuth, navigate]
  );

  const handleChange = useCallback(
    (value: string) => {
      setName(value);
      setError(null);
    },
    []
  );

  return (
    <div className="flex flex-col flex-1 bg-background">
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
            aria-invalid={!!displayNameError || undefined}
            aria-describedby={displayNameError ? "organization-name-error" : undefined}
          />
          {displayNameError && (
            <p id="organization-name-error" className="mt-1.5 text-xs text-destructive">
              {displayNameError}
            </p>
          )}
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
          disabled={createAccount.isPending}
          className="w-full"
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
  return <OrganizationNewContent />;
}
