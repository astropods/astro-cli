import { type ReactNode } from "react";
import { BlueprintCard } from "@/components/BlueprintCard";
import { getBlueprintDescription } from "@/lib/blueprint-utils";
import type { Blueprint } from "@/lib/api";

function BlueprintCardSkeleton() {
  return (
    <div className="relative overflow-hidden rounded-sm border border-stone-300 bg-background animate-pulse dark:border-border">
      <div className="flex items-start gap-3 p-4 pb-3">
        <div className="size-9 shrink-0 rounded-[3px] bg-muted" />
        <div className="flex-1 space-y-2 pt-0.5">
          <div className="h-4 w-32 rounded bg-muted" />
          <div className="h-3 w-full rounded bg-muted" />
          <div className="h-3 w-3/4 rounded bg-muted" />
        </div>
      </div>
      <div className="mx-[5px] border-t border-stone-200 dark:border-border" />
      <div className="flex items-center justify-between px-4 py-2.5 pb-3.5">
        <div className="h-3 w-14 rounded bg-muted" />
        <div className="h-3 w-20 rounded bg-muted" />
      </div>
    </div>
  );
}

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
  skeletonCount?: number;
  showAuthor?: boolean;
  /** Forwarded to each BlueprintCard so the blueprint-detail breadcrumb can
   *  reflect the surface the user came from. */
  from?: string;
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
  skeletonCount = 6,
  showAuthor = false,
  from,
}: BlueprintListViewProps) {
  if (isLoading) {
    return (
      <div
        role="status"
        aria-label="Loading blueprints"
        className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3 @[1200px]:grid-cols-4 content-start"
      >
        {Array.from({ length: skeletonCount }).map((_, i) => (
          <BlueprintCardSkeleton key={i} />
        ))}
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
            avatarColors={blueprint.avatar_colors}
            deployCount={blueprint.metrics?.deploy_count}
            heartCount={blueprint.heart_count}
            isDraft={blueprint.versions.length === 0}
            onArchive={ownerAccounts?.has(blueprint.account) ? () => {} : undefined}
            author={showAuthor ? blueprint.publishers?.[0] : undefined}
            from={from}
          />
        ))}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3 @[1200px]:grid-cols-4 content-start">
      {sorted.map((blueprint) => (
        <BlueprintCard
          key={`${blueprint.account}/${blueprint.name}`}
          slug={`${blueprint.account}/${blueprint.name}`}
          account={blueprint.account}
          name={blueprint.name}
          description={getBlueprintDescription(blueprint)}
          visibility={blueprint.visibility}
          avatarColors={blueprint.avatar_colors}
          deployCount={blueprint.metrics?.deploy_count}
          isDraft={blueprint.versions.length === 0}
          onArchive={ownerAccounts?.has(blueprint.account) ? () => {} : undefined}
          author={showAuthor ? blueprint.publishers?.[0] : undefined}
          from={from}
        />
      ))}
    </div>
  );
}
