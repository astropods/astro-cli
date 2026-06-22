import { useMemo, useState } from "react";
import { useBlueprints } from "@/api/queries";
import { createServerApi } from "@/lib/api.server";
import { BlueprintListView } from "@/components/browse/BlueprintListView";
import { FilterInput } from "@/components/FilterInput";
import { PageContainer, PageHeader } from "@/components/PageLayout";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuth } from "@/lib/auth";
import { explorePath } from "@/lib/routes";
import { getBlueprintCategories, getBlueprintDescription } from "@/lib/blueprint-utils";
import type { Blueprint } from "@/lib/api";

type ExploreSort = "deploys" | "hearts" | "updated" | "name";

const EXPLORE_SORT_OPTIONS: { value: ExploreSort; label: string }[] = [
  { value: "deploys", label: "Most deploys" },
  { value: "hearts", label: "Most hearted" },
  { value: "updated", label: "Last updated" },
  { value: "name", label: "Name (A-Z)" },
];

function blueprintName(a: Blueprint) {
  return `${a.name} ${a.account}`;
}

function compareByName(a: Blueprint, b: Blueprint) {
  return blueprintName(a).localeCompare(blueprintName(b), undefined, { sensitivity: "base" });
}

function compareMetricDesc(
  a: Blueprint,
  b: Blueprint,
  getMetric: (blueprint: Blueprint) => number,
) {
  const metricDelta = getMetric(b) - getMetric(a);
  if (metricDelta !== 0) return metricDelta;
  return compareByName(a, b);
}

function latestPublishedAt(blueprint: Blueprint) {
  return blueprint.versions.reduce(
    (latest, version) => (version.published_at > latest ? version.published_at : latest),
    "",
  );
}

function matchesSearch(blueprint: Blueprint, search: string) {
  const q = search.trim().toLowerCase();
  if (!q) return true;

  const categories = getBlueprintCategories(blueprint).join(" ");
  const haystack = [
    blueprint.name,
    blueprint.account,
    getBlueprintDescription(blueprint),
    categories,
  ].join(" ").toLowerCase();

  return haystack.includes(q);
}

function sortExploreBlueprints(blueprints: Blueprint[], sort: ExploreSort) {
  return [...blueprints].sort((a, b) => {
    if (sort === "name") return compareByName(a, b);
    if (sort === "updated") {
      const updatedDelta = latestPublishedAt(b).localeCompare(latestPublishedAt(a));
      if (updatedDelta !== 0) return updatedDelta;
      return compareByName(a, b);
    }
    if (sort === "hearts") {
      return compareMetricDesc(a, b, (blueprint) => blueprint.heart_count ?? 0);
    }
    return compareMetricDesc(a, b, (blueprint) => blueprint.metrics?.deploy_count ?? 0);
  });
}

export async function loader({ request }: { request: Request }) {
  const api = createServerApi(request);
  const blueprintsData = await api.listBlueprints().catch(() => ({ agents: [], count: 0 }));
  return { blueprintsData };
}

export function meta() {
  return [
    { title: "Explore | Astro" },
    { name: "description", content: "Explore public AI agent blueprints on Astro." },
    { property: "og:title", content: "Explore | Astro" },
    { property: "og:description", content: "Explore public AI agent blueprints on Astro." },
  ];
}

export default function Explore({ loaderData }: { loaderData: Awaited<ReturnType<typeof loader>> }) {
  const [sort, setSort] = useState<ExploreSort>("deploys");
  const [search, setSearch] = useState("");
  const { data, isLoading, isError, error, refetch } = useBlueprints({
    initialData: loaderData?.blueprintsData,
  });
  const { accounts } = useAuth();
  const ownerAccounts = new Set(accounts.map((a) => a.name));
  const allBlueprints = data?.agents ?? [];
  const blueprints = useMemo(
    () => sortExploreBlueprints(
      allBlueprints.filter((blueprint) => matchesSearch(blueprint, search)),
      sort,
    ),
    [allBlueprints, search, sort],
  );
  const hasFilters = search.trim() !== "";

  return (
    <PageContainer outerClassName="bg-background">
      <PageHeader
        title="Explore"
        description="Public agent configurations available to deploy in your account or organization."
      />
      <div className="mb-4 flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <FilterInput
            placeholder="Search blueprints..."
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            containerClassName="h-8 w-full bg-card dark:bg-background @[480px]:w-auto @[480px]:max-w-lg @[480px]:flex-1"
          />
          <div className="flex w-full flex-wrap items-center gap-2 @[480px]:w-auto">
            <Select
              value={sort}
              onValueChange={(value) => setSort(value as ExploreSort)}
            >
              <SelectTrigger className="h-8 w-full bg-card px-3 text-sm dark:bg-background @[480px]:w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {EXPLORE_SORT_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>
      <BlueprintListView
        blueprints={blueprints}
        isLoading={isLoading}
        isError={isError}
        error={error}
        refetch={refetch}
        emptyTitle={hasFilters ? "No blueprints match your filters" : "No blueprints available"}
        emptyDescription={
          hasFilters
            ? "Try a different search or category."
            : "There are no blueprints in the registry yet."
        }
        ownerAccounts={ownerAccounts}
        from={explorePath}
      />
    </PageContainer>
  );
}
