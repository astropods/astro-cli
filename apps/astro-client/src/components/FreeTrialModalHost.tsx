/** Opens once after account creation and never again once closed.
 *  `?freeTrial=1` is a manual QA override. */
import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { useActiveAccount } from "@/hooks/use-active-account";
import { useAuth } from "@/lib/auth";
import { useBillingBalances } from "@/api/queries/billing";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { usePendingFreeTrialModal } from "@/hooks/use-pending-free-trial-modal";
import { creditUnit, toBalanceRow } from "@/lib/billing-balances";
import { blueprintsAccountPath, explorePath } from "@/lib/routes";
import { FreeTrialModal } from "@/components/FreeTrialModal";

export function FreeTrialModalHost() {
  const [searchParams, setSearchParams] = useSearchParams();
  const { user } = useAuth();
  const { pending, clearPending } = usePendingFreeTrialModal(user?.id);
  const override = searchParams.get("freeTrial") === "1";

  if (!pending && !override) return null;

  const close = () => {
    clearPending();
    if (override) {
      const next = new URLSearchParams(searchParams);
      next.delete("freeTrial");
      setSearchParams(next, { replace: true });
    }
  };

  return <PendingFreeTrialModal pending={pending} clearPending={clearPending} onClose={close} />;
}

function usdGrant(creditType?: string, creditTypeId?: string): boolean {
  const unit = creditUnit(creditType, creditTypeId);
  return unit.kind === "money" && unit.currency === "USD";
}

// The signup credit is granted by a queued job, so a read firing the moment
// the account is created can legitimately see nothing yet. Exported so tests
// drive the loop rather than copying these numbers.
export const NO_GRANT_RETRY_MS = 3000;
export const NO_GRANT_MAX_RETRIES = 5;

function PendingFreeTrialModal({
  pending,
  clearPending,
  onClose,
}: {
  pending: boolean;
  clearPending: () => void;
  onClose: () => void;
}) {
  const { activeAccount } = useActiveAccount();
  const navigate = useNavigate();
  const { data, isLoading, isError, refetch } = useBillingBalances(activeAccount);
  const { data: blueprintsData } = useAccountBlueprints(activeAccount);
  const hasBlueprints = (blueprintsData?.agents?.length ?? 0) > 0;
  const [retries, setRetries] = useState(0);

  // What was granted, not what is left. A grant in another unit has no dollar
  // figure to announce.
  const rows = data?.available ? (data.data?.credits ?? []).map(toBalanceRow) : [];
  const creditRow = rows.find((r) => (r.granted ?? 0) > 0 && usdGrant(r.creditType, r.creditTypeId));
  const unit = creditUnit(creditRow?.creditType, creditRow?.creditTypeId);
  // A row that exists but does not qualify is everything the provider has;
  // retrying cannot change it.
  const nonQualifyingGrant = pending && !isLoading && !isError && rows.length > 0 && !creditRow;
  // Answered, and the answer was none. An account on the no-credit package
  // never gets a grant, so this has to resolve rather than re-query forever.
  const answeredNoCredits =
    pending && !isLoading && !isError && !creditRow && data?.available === true && rows.length === 0;
  // Never answered. available:false also covers a failed customer resolve,
  // which runs on first billing access, exactly when this modal fires. Neither
  // is evidence of no grant, so the window closes without spending the flag.
  const unanswered =
    pending && !isLoading && !creditRow && (isError || data?.available === false);
  const retriesExhausted = retries >= NO_GRANT_MAX_RETRIES;
  const stillWaiting = answeredNoCredits || unanswered;
  const noGrant = nonQualifyingGrant || (answeredNoCredits && retriesExhausted);

  useEffect(() => {
    if (!stillWaiting || retriesExhausted) return;
    const t = setTimeout(() => {
      setRetries((n) => n + 1);
      refetch();
    }, NO_GRANT_RETRY_MS);
    return () => clearTimeout(t);
    // retries is a dep so each increment re-arms the next timer.
  }, [stillWaiting, retriesExhausted, refetch, retries]);

  // Clearing writes localStorage and notifies subscribers, so it can't run
  // during the render that discovers there's no grant.
  useEffect(() => {
    if (!noGrant) return;
    clearPending();
  }, [noGrant, clearPending]);

  if (isLoading || !creditRow) return null;

  return (
    <FreeTrialModal
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      credits={(creditRow.granted ?? 0) / (unit.kind === "money" ? unit.scale : 1)}
      onCta={() => {
        onClose();
        navigate(hasBlueprints ? blueprintsAccountPath(activeAccount) : explorePath);
      }}
    />
  );
}
