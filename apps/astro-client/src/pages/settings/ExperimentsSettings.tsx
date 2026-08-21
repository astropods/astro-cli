import type { MetaFunction } from "react-router";
import { Navigate } from "react-router";
import { hasExperiments, useExperiments } from "@/lib/experiments";
import { Switch } from "@/components/ui/switch";
import { SectionHeader } from "@/components/settings/SettingsShared";
import { useAuth } from "@/lib/auth";
import {
  useAccountExperiment,
  useUpdateAccountExperiment,
  PROMPT_CLASSIFICATION_STATS,
} from "@/api/queries";
import { getApiErrorMessage } from "@/lib/api";

export const meta: MetaFunction = () => [{ title: "Experiments - Settings | Astro" }];

export default function ExperimentsSettings() {
  const { experiments, setExperiment } = useExperiments();
  // Server-owned switches belong to an account, and on this page that is the
  // reader's personal one. Organizations set theirs in organization settings,
  // where the switch applies to every member.
  const personalAccount = useAuth().personalAccount?.name ?? "";
  const classification = useAccountExperiment(personalAccount, PROMPT_CLASSIFICATION_STATS);
  const updateClassification = useUpdateAccountExperiment(personalAccount, PROMPT_CLASSIFICATION_STATS);

  // The server-owned switch below is not part of the localStorage registry, so
  // retiring the last local toggle must not redirect it away.
  if (!hasExperiments && !personalAccount) return <Navigate to="/settings/account" replace />;
  return (
    <>
      <SectionHeader title="Experiments" subtitle="Early-access features that may change or be removed" />
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4 rounded-md border border-border px-4 py-3">
          <div>
            <p className="text-body-sm font-medium text-foreground">Eval tab</p>
            <p className="text-body-sm text-muted-foreground">Show the Eval tab on agent detail pages.</p>
          </div>
          <Switch
            checked={experiments.evals}
            onCheckedChange={(checked) => setExperiment("evals", checked)}
          />
        </div>
        <div className="flex items-center justify-between gap-4 rounded-md border border-border px-4 py-3">
            <div>
              <p className="text-body-sm font-medium text-foreground">Prompt classification statistics</p>
              <p className="text-body-sm text-muted-foreground">
                Categorise your coding-tool prompts by purpose and topic, and show the breakdown on
                a detail page from Insights. Classification is experimental and may not always be
                accurate.
              </p>
            </div>
          <Switch
            aria-label="Prompt classification statistics"
            checked={classification.data?.enabled ?? false}
            disabled={!personalAccount || classification.isLoading || classification.isError || updateClassification.isPending}
            onCheckedChange={(enabled) => updateClassification.mutate(enabled)}
          />
        </div>
        {(classification.error ?? updateClassification.error) && (
          <p role="alert" className="text-body-sm text-destructive">
            {getApiErrorMessage(
              classification.error ?? updateClassification.error,
              "Could not update prompt classification statistics.",
            )}
          </p>
        )}
      </div>
    </>
  );
}
