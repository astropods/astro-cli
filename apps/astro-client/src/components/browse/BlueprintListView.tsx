import { type ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { BlueprintCard } from "@/components/BlueprintCard";
import { getBlueprintDescription } from "@/lib/blueprint-utils";
import type { Blueprint } from "@/lib/api";

export interface BlueprintListViewProps {
  blueprints: Blueprint[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  emptyTitle?: string;
  emptyDescription?: string;
  emptyContent?: ReactNode;
  ownerAccounts?: Set<string>;
  variant?: "grid" | "list";
}

export function BlueprintListView({
  blueprints,
  isLoading,
  isError,
  error,
  refetch,
  emptyTitle = "No blueprints yet",
  emptyDescription = "There are no blueprints in the registry yet.",
  emptyContent,
  ownerAccounts,
  variant = "grid",
}: BlueprintListViewProps) {
  if (isLoading) {
    return (
      <div role="status" aria-label="Loading blueprints" className="flex items-center justify-center py-12">
        <Loader2 size={32} className="animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-700">
        <p className="font-medium">Failed to load blueprints</p>
        <p className="text-sm">
          {(error as { error_description?: string })?.error_description ??
            (error instanceof Error ? error.message : "An unexpected error occurred")}
        </p>
        <button
          type="button"
          onClick={() => refetch()}
          className="mt-2 cursor-pointer rounded-md border border-red-300 bg-white px-3 py-1 text-sm text-red-700 hover:bg-red-50"
        >
          Retry
        </button>
      </div>
    );
  }

  if (blueprints.length === 0) {
    if (emptyContent) return <>{emptyContent}</>;
    return (
      <div className="rounded-lg border border-border p-8 text-center">
        <h3 className="mb-2 text-lg font-medium">{emptyTitle}</h3>
        <p className="text-sm text-muted-foreground">
          {emptyDescription}{" "}
          <a
            href="https://docs.astropods.com/publish-to-registry"
            target="_blank"
            rel="noopener noreferrer"
            className="underline text-primary hover:text-primary/70"
          >
            Learn how to push a blueprint
          </a>
        </p>
      </div>
    );
  }

  const sorted = [...blueprints].sort((a, b) => {
    const aDraft = a.versions.length === 0 ? 0 : 1;
    const bDraft = b.versions.length === 0 ? 0 : 1;
    return aDraft - bDraft;
  });

  if (variant === "list") {
    return (
      <div className="flex flex-col gap-2">
        {sorted.map((blueprint) => (
          <BlueprintCard
            key={`${blueprint.account}/${blueprint.name}`}
            variant="list"
            slug={`${blueprint.account}/${blueprint.name}`}
            account={blueprint.account}
            name={blueprint.name}
            description={getBlueprintDescription(blueprint)}
            visibility={blueprint.visibility}
            avatarUrl={blueprint.avatar_url}
            deployCount={blueprint.metrics?.deploy_count}
            heartCount={blueprint.heart_count}
            isDraft={blueprint.versions.length === 0}
            onArchive={ownerAccounts?.has(blueprint.account) ? () => {} : undefined}
          />
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3 content-start">
      {sorted.map((blueprint) => (
        <BlueprintCard
          key={`${blueprint.account}/${blueprint.name}`}
          slug={`${blueprint.account}/${blueprint.name}`}
          account={blueprint.account}
          name={blueprint.name}
          description={getBlueprintDescription(blueprint)}
          visibility={blueprint.visibility}
          deployCount={blueprint.metrics?.deploy_count}
          isDraft={blueprint.versions.length === 0}
          onArchive={ownerAccounts?.has(blueprint.account) ? () => {} : undefined}
        />
      ))}
    </div>
  );
}
