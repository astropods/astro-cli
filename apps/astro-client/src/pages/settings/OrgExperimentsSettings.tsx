import { useParams, type MetaFunction } from "react-router";
import { Switch } from "@/components/ui/switch";
import { SectionHeader } from "@/components/settings/SettingsShared";
import {
  useFineGrainedAccessExperiment,
  useUpdateFineGrainedAccessExperiment,
} from "@/api/queries";
import { getApiErrorMessage } from "@/lib/api";

export const meta: MetaFunction = () => [
  { title: "Experiments - Organization Settings | Astro" },
];

export default function OrgExperimentsSettings() {
  const { orgSlug = "" } = useParams();
  const experiment = useFineGrainedAccessExperiment(orgSlug);
  const updateExperiment = useUpdateFineGrainedAccessExperiment(orgSlug);
  const error = experiment.error ?? updateExperiment.error;

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
              creators become editors, and other members need an assigned deployment role.
            </p>
          </div>
          <Switch
            aria-label="Fine-grained access"
            checked={experiment.data?.enabled ?? false}
            disabled={experiment.isLoading || experiment.isError || updateExperiment.isPending}
            onCheckedChange={(enabled) => updateExperiment.mutate(enabled)}
          />
        </div>
        {error && (
          <p role="alert" className="text-body-sm text-destructive">
            {getApiErrorMessage(error, "Could not update fine-grained access.")}
          </p>
        )}
      </div>
    </>
  );
}
