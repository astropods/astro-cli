import { useParams, type MetaFunction } from "react-router";
import { Switch } from "@/components/ui/switch";
import { SectionHeader } from "@/components/settings/SettingsShared";
import {
  useAccountExperiment,
  useUpdateAccountExperiment,
  FINE_GRAINED_ACCESS,
  PROMPT_CLASSIFICATION_STATS,
} from "@/api/queries";
import { getApiErrorMessage } from "@/lib/api";

export const meta: MetaFunction = () => [
  { title: "Experiments - Organization Settings | Astro" },
];

export default function OrgExperimentsSettings() {
  const { orgSlug = "" } = useParams();
  const experiment = useAccountExperiment(orgSlug, FINE_GRAINED_ACCESS);
  const updateExperiment = useUpdateAccountExperiment(orgSlug, FINE_GRAINED_ACCESS);
  const classification = useAccountExperiment(orgSlug, PROMPT_CLASSIFICATION_STATS);
  const updateClassification = useUpdateAccountExperiment(orgSlug, PROMPT_CLASSIFICATION_STATS);
  const error = experiment.error ?? updateExperiment.error
    ?? classification.error ?? updateClassification.error;

  return (
    <>
      <SectionHeader
        title="Experiments"
        subtitle="Early-access organization features that may change or be removed"
      />
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4 rounded-md border border-border px-4 py-3">
          <div>
            <p className="text-body-sm font-medium text-foreground">Fine-grained access</p>
            <p className="text-body-sm text-muted-foreground">
              Make synchronized deployments private by default. Owners and admins retain access,
              creators become deployment owners, and other members need an assigned deployment role.
            </p>
          </div>
          <Switch
            aria-label="Fine-grained access"
            checked={experiment.data?.enabled ?? false}
            disabled={experiment.isLoading || experiment.isError || updateExperiment.isPending}
            onCheckedChange={(enabled) => updateExperiment.mutate(enabled)}
          />
        </div>
        <div className="flex items-center justify-between gap-4 rounded-md border border-border px-4 py-3">
          <div>
            <p className="text-body-sm font-medium text-foreground">Prompt classification statistics</p>
            <p className="text-body-sm text-muted-foreground">
              Categorise coding-tool prompts by purpose and topic, and show the breakdown on a
              detail page from Insights. Classification is experimental and may not always be
              accurate.
            </p>
          </div>
          <Switch
            aria-label="Prompt classification statistics"
            checked={classification.data?.enabled ?? false}
            disabled={classification.isLoading || classification.isError || updateClassification.isPending}
            onCheckedChange={(enabled) => updateClassification.mutate(enabled)}
          />
        </div>
        {error && (
          <p role="alert" className="text-body-sm text-destructive">
            {getApiErrorMessage(error, "Could not update experiments.")}
          </p>
        )}
      </div>
    </>
  );
}
