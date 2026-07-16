import { useMemo } from "react";
import { AlertCircle, Loader2 } from "lucide-react";
import { DeploymentStatusBadge } from "@/components/agent-detail/deployments/DeploymentStatusBadge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import { commitTitle, shortSha } from "@/lib/github-utils";
import { formatRelativeTime, shortBuildId } from "@/lib/deployment-utils";
import { getApiErrorMessage, type BlueprintVersion } from "@/lib/api";
import { FormSection } from "./FormSection";

interface BlueprintVersionPickerProps {
  versions: BlueprintVersion[];
  selectedBuildId: string;
  latestBuildId?: string;
  currentBuildId?: string;
  onBuildChange: (buildId: string) => void;
  loading?: boolean;
  error?: unknown;
  recovery?: { label: string; onClick: () => void };
}

function LatestBadge() {
  return (
    <span className="flex items-center gap-1.5 rounded-full bg-[color-mix(in_oklch,var(--color-indigo-600)_16%,transparent)] px-2 py-0.5 text-mono-sm font-medium text-indigo-700 dark:bg-[color-mix(in_oklch,var(--color-indigo-400)_22%,transparent)] dark:text-indigo-300">
      Latest
    </span>
  );
}

function NewBuildAvailableIndicator() {
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 text-mono-sm font-medium text-primary dark:text-indigo-300">
      <span className="size-1.5 rounded-full bg-primary shadow-[0_0_6px_2px] shadow-primary/50 dark:bg-indigo-300 dark:shadow-indigo-300/50" />
      New build available
    </span>
  );
}

function buildOptions(
  versions: BlueprintVersion[],
  selectedBuildId: string,
  latestBuildId?: string,
  currentBuildId?: string,
) {
  const options = [...versions].sort(
    (a, b) => Date.parse(b.published_at) - Date.parse(a.published_at),
  );
  const known = new Set(options.map(({ build_id }) => build_id));
  for (const buildId of [latestBuildId, currentBuildId, selectedBuildId]) {
    if (buildId && !known.has(buildId)) {
      options.push({ build_id: buildId, published_at: "", spec: {} });
      known.add(buildId);
    }
  }
  return latestBuildId
    ? options.sort((a, b) => Number(b.build_id === latestBuildId) - Number(a.build_id === latestBuildId))
    : options;
}

function VersionSummary({
  version,
  latestBuildId,
  currentBuildId,
  selected = false,
}: {
  version: BlueprintVersion;
  latestBuildId?: string;
  currentBuildId?: string;
  selected?: boolean;
}) {
  const title = commitTitle(version.commit_message)
    ?? (version.version ? `Version ${version.version}` : `Build ${shortBuildId(version.build_id)}`);
  const buildLabel = shortSha(version.commit_sha) ?? shortBuildId(version.build_id);
  const showBuildLabel = title !== `Build ${buildLabel}`;
  const description = version.commit_message
    ?.split("\n")
    .slice(1)
    .map((line) => line.trim())
    .find(Boolean);
  const isLatest = version.build_id === latestBuildId;
  const isCurrent = version.build_id === currentBuildId;
  const relativeTime = version.published_at ? formatRelativeTime(version.published_at) : undefined;
  const timestamp = relativeTime ? `Pushed ${relativeTime}` : undefined;

  return (
    <span className="flex min-w-0 flex-1 items-start gap-3 text-left">
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="truncate text-[13px] font-medium leading-5 text-foreground">{title}</span>
          {showBuildLabel ? <span className="shrink-0 font-mono text-body-sm text-faint-foreground">{buildLabel}</span> : null}
          {isLatest && <LatestBadge />}
          {isCurrent && <DeploymentStatusBadge status="active" label="Current" />}
        </span>
        {description ? (
          <span className="mt-0.5 block truncate text-body-sm text-muted-foreground">{description}</span>
        ) : null}
      </span>
      {timestamp ? <span className="ml-auto shrink-0 pt-0.5 text-[11px] leading-4 text-faint-foreground">{timestamp}</span> : null}
      {selected ? <span className="sr-only">Selected</span> : null}
    </span>
  );
}

function SwitchError({ error, recovery }: Pick<BlueprintVersionPickerProps, "error" | "recovery">) {
  return error ? (
    <div role="alert" className="mt-3 flex items-start gap-3 rounded-md border border-destructive/25 bg-destructive/8 px-3.5 py-3 text-body-sm text-foreground">
      <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
      <div className="min-w-0 flex-1">
        <p className="font-medium">Couldn’t load this build</p>
        <p className="mt-0.5 text-muted-foreground">
          {getApiErrorMessage(error, "The fields below still reflect the previously loaded build.")}
        </p>
      </div>
      {recovery && <Button type="button" variant="outline" size="sm" onClick={recovery.onClick}>{recovery.label}</Button>}
    </div>
  ) : null;
}

export function BlueprintVersionPicker({
  versions,
  selectedBuildId,
  latestBuildId,
  currentBuildId,
  onBuildChange,
  loading = false,
  error,
  recovery,
}: BlueprintVersionPickerProps) {
  const options = useMemo(
    () => buildOptions(versions, selectedBuildId, latestBuildId, currentBuildId),
    [versions, selectedBuildId, latestBuildId, currentBuildId],
  );
  const selected = options.find(({ build_id }) => build_id === selectedBuildId);
  if (!selected) return null;
  const hasNewBuild = !!latestBuildId && !!currentBuildId && latestBuildId !== currentBuildId;

  const summary = (version: BlueprintVersion, isSelected = false) => (
    <VersionSummary
      version={version}
      latestBuildId={latestBuildId}
      currentBuildId={currentBuildId}
      selected={isSelected}
    />
  );
  return (
    <div className="mb-12">
      <FormSection
        title="Blueprint version"
        description="Select the published build this agent should run."
        action={hasNewBuild ? <NewBuildAvailableIndicator /> : null}
      >
        <Select value={selectedBuildId} onValueChange={onBuildChange} disabled={loading}>
          <SelectTrigger
            aria-label="Blueprint version"
            className="h-auto min-h-11 gap-3 py-2 pr-3 [&>span]:line-clamp-none [&>span]:flex [&>span]:min-w-0 [&>span]:flex-1"
          >
            {summary(selected)}
            {loading ? <Loader2 className="size-4 shrink-0 animate-spin text-muted-foreground" /> : null}
          </SelectTrigger>
          <SelectContent className="max-h-80">
            {options.map((version) => {
              const title = commitTitle(version.commit_message)
                ?? (version.version ? `Version ${version.version}` : `Build ${shortBuildId(version.build_id)}`);
              return (
                <SelectItem
                  key={version.build_id}
                  value={version.build_id}
                  textValue={title}
                  className="items-center pr-3 pl-8 [&>span:last-child]:min-w-0 [&>span:last-child]:flex-1"
                >
                  {summary(version, version.build_id === selectedBuildId)}
                </SelectItem>
              );
            })}
          </SelectContent>
        </Select>
        <SwitchError error={error} recovery={recovery} />
      </FormSection>
    </div>
  );
}
