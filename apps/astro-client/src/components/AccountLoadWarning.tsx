import { useMemo } from "react";
import { ActionPanel } from "@/components/ui/status-panel";
import { useAuth } from "@/lib/auth";

interface AccountLoadWarningProps {
  failedAccounts: string[];
  onRetry: () => void;
}

function formatAccounts(names: string[]) {
  const shown = names.slice(0, 3).join(", ");
  return names.length > 3 ? `${shown}, and ${names.length - 3} more` : shown;
}

export function AccountLoadWarning({ failedAccounts, onRetry }: AccountLoadWarningProps) {
  const { accounts } = useAuth();
  const labels = useMemo(
    () => new Map(accounts.map((account) => [account.name, account.display_name || account.name])),
    [accounts],
  );

  if (failedAccounts.length === 0) return null;
  const names = failedAccounts.map((name) => labels.get(name) ?? name);
  const list = formatAccounts(names);
  const title =
    failedAccounts.length === 1
      ? `Couldn't load ${list}`
      : `Couldn't load ${failedAccounts.length} accounts`;

  return (
    <div role="alert">
      <ActionPanel
        tone="warning"
        title={title}
        primaryLabel="Retry"
        onPrimary={onRetry}
      >
        {failedAccounts.length === 1
          ? "Some items may be missing from this list."
          : `Items from ${list} may be missing from this list.`}
      </ActionPanel>
    </div>
  );
}
