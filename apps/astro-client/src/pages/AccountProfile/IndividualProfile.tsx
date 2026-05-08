import { useState, useMemo, useEffect, useCallback } from "react";
import { useParams } from "react-router";
import type { Route } from "./+types/AccountProfile";
import { useAccount, useAccountOrgs, useUpdateAccountProfile } from "@/api/queries/accounts";
import { useDeployments } from "@/api/queries/deployments";
import { useAuth } from "@/lib/auth";
import { useAccountBlueprints } from "@/api/queries/blueprints";
import { useHeartedBlueprints } from "@/api/queries/hearts";
import { PageContainer } from "@/components/PageLayout";
import { GradientGridWash } from "@/components/GradientGridWash";
import { Button } from "@/components/ui/button";
import { Eye } from "lucide-react";
import { cn } from "@/lib/utils";
import { ProfileEditSidebar } from "./ProfileEditSidebar";
import { ProfileViewSidebar } from "./ProfileViewSidebar";
import { BlueprintsTab } from "./BlueprintsTab";
import { AgentsTab } from "./AgentsTab";
import { HeartsTab } from "./HeartsTab";
import type { VisibilityFilter, BlueprintSort, ReorderMode } from "./BlueprintsTab";
import type { AgentSort } from "./AgentsTab";
import type { HeartSort } from "./HeartsTab";

// ── Types ─────────────────────────────────────────────────────────────────────

type ViewMode = "internal" | "external";
type Tab = "blueprints" | "agents" | "hearts";

// ── Helpers ───────────────────────────────────────────────────────────────────

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "pb-3 text-body border-b-2 -mb-px transition-colors cursor-pointer",
        active
          ? "border-primary text-foreground font-semibold"
          : "border-transparent text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

// ── IndividualProfile ─────────────────────────────────────────────────────────

interface IndividualProfileProps {
  loaderData: Route.ComponentProps["loaderData"];
}

export function IndividualProfile({ loaderData }: IndividualProfileProps) {
  const { account } = useParams<{ account: string }>();
  const { data } = useAccount(account ?? "", {
    initialData: loaderData.account ?? undefined,
  });
  const { isAuthenticated, accounts } = useAuth();
  const { data: orgsData } = useAccountOrgs(account ?? "", {
    initialData: loaderData.orgs ?? undefined,
  });

  const isOwner = isAuthenticated && accounts.some((a) => a.name === data?.name);

  // ── View mode ──────────────────────────────────────────────────────────────
  const [viewMode, setViewMode] = useState<ViewMode>("internal");
  const effectiveViewMode: ViewMode = isOwner ? viewMode : "external";

  // ── Sidebar edit ──────────────────────────────────────────────────────────
  const [editOpen, setEditOpen] = useState(false);

  // ── Tab ───────────────────────────────────────────────────────────────────
  const [activeTab, setActiveTab] = useState<Tab>("blueprints");

  // ── Blueprint filters ─────────────────────────────────────────────────────
  const [bpSearch, setBpSearch] = useState("");
  const [bpVisibility, setBpVisibility] = useState<VisibilityFilter>("all");
  const [bpSort, setBpSort] = useState<BlueprintSort>("newest");

  // ── Blueprint reorder ─────────────────────────────────────────────────────
  const updateProfile = useUpdateAccountProfile();
  const [bpReorderMode, setBpReorderMode] = useState<ReorderMode>("idle");
  const [bpCustomOrder, setBpCustomOrder] = useState<string[] | null>(null);

  // ── Agent filters ─────────────────────────────────────────────────────────
  const [agentSearch, setAgentSearch] = useState("");
  const [agentSort, setAgentSort] = useState<AgentSort>("modified");

  // ── Hearts filters ────────────────────────────────────────────────────────
  const [heartSearch, setHeartSearch] = useState("");
  const [heartSort, setHeartSort] = useState<HeartSort>("newest");
  const { data: heartsData } = useHeartedBlueprints(data?.name ?? "", undefined, {
    enabled: !!data,
    initialData: loaderData.hearts ?? undefined,
  });

  // ── Data ──────────────────────────────────────────────────────────────────
  const { data: deploymentsData } = useDeployments(data?.name ?? "", isOwner, {
    initialData: loaderData.deployments ?? undefined,
  });
  const { data: blueprintsData } = useAccountBlueprints(data?.name ?? "", {
    enabled: !!data,
    initialData: loaderData.blueprints ?? undefined,
  });

  const orgs = orgsData?.orgs ?? [];
  const rawBlueprints = useMemo(() => blueprintsData?.agents ?? [], [blueprintsData]);
  const rawDeployments = useMemo(() => deploymentsData?.deployments ?? [], [deploymentsData]);

  // ── Seed custom blueprint order from profile ──────────────────────────────
  useEffect(() => {
    if (data?.blueprint_order) setBpCustomOrder(data.blueprint_order);
  }, [data?.blueprint_order]);

  // ── Reorder handlers ──────────────────────────────────────────────────────
  const handleEnterReorder = useCallback(() => {
    // Reset all filters so reordering operates on the full set
    setBpSearch("");
    setBpVisibility("all");
    setBpSort("newest");
    setBpReorderMode("editing");
  }, []);

  const handleSaveReorder = useCallback((names: string[]) => {
    if (!data?.name) return;
    const prevOrder = bpCustomOrder;
    setBpCustomOrder(names);
    setBpReorderMode("saved");
    updateProfile.mutate(
      { account: data.name, blueprint_order: names },
      {
        onSuccess: () => setTimeout(() => setBpReorderMode("idle"), 1500),
        onError: () => {
          setBpCustomOrder(prevOrder);
          setBpReorderMode("editing");
        },
      },
    );
  }, [data?.name, bpCustomOrder, updateProfile]);

  // ── Blueprint filtering + sorting ─────────────────────────────────────────
  const visibleBlueprints = useMemo(() => {
    let list = effectiveViewMode === "external"
      ? rawBlueprints.filter((bp) => bp.visibility === "public")
      : rawBlueprints;

    if (effectiveViewMode === "internal" && bpVisibility !== "all") {
      list = list.filter((bp) => bp.visibility === bpVisibility);
    }

    if (bpSearch.trim()) {
      const q = bpSearch.toLowerCase();
      list = list.filter((bp) => bp.name.toLowerCase().includes(q));
    }

    if (bpSort === "newest" && bpCustomOrder) {
      // Apply saved custom order; new blueprints not in order fall to end by newest
      const orderMap = new Map(bpCustomOrder.map((name, i) => [name, i]));
      list = [...list].sort((a, b) => {
        const ai = orderMap.get(a.name) ?? Infinity;
        const bi = orderMap.get(b.name) ?? Infinity;
        if (ai !== bi) return ai - bi;
        const latestA = a.versions.reduce((m, v) => (v.published_at > m ? v.published_at : m), "");
        const latestB = b.versions.reduce((m, v) => (v.published_at > m ? v.published_at : m), "");
        return latestB.localeCompare(latestA);
      });
    } else if (bpSort === "newest") {
      list = [...list].sort((a, b) => {
        const latestA = a.versions.reduce((m, v) => (v.published_at > m ? v.published_at : m), "");
        const latestB = b.versions.reduce((m, v) => (v.published_at > m ? v.published_at : m), "");
        return latestB.localeCompare(latestA);
      });
    } else if (bpSort === "name") {
      list = [...list].sort((a, b) => a.name.localeCompare(b.name));
    } else if (bpSort === "deployed") {
      list = [...list].sort((a, b) => (b.metrics?.deploy_count ?? 0) - (a.metrics?.deploy_count ?? 0));
    }

    return list;
  }, [rawBlueprints, effectiveViewMode, bpVisibility, bpSearch, bpSort, bpCustomOrder]);

  // ── Agent filtering + sorting ─────────────────────────────────────────────
  const visibleDeployments = useMemo(() => {
    let list = rawDeployments;

    if (agentSearch.trim()) {
      const q = agentSearch.toLowerCase();
      list = list.filter((d) =>
        d.name.toLowerCase().includes(q) ||
        (d.display_name ?? "").toLowerCase().includes(q),
      );
    }

    if (agentSort === "modified") {
      list = [...list].sort((a, b) => {
        const aDate = a.updated_at || a.created_at;
        const bDate = b.updated_at || b.created_at;
        return bDate.localeCompare(aDate);
      });
    } else if (agentSort === "name") {
      list = [...list].sort((a, b) => a.name.localeCompare(b.name));
    }

    return list;
  }, [rawDeployments, agentSearch, agentSort]);

  if (!data) {
    return (
      <div className="flex flex-1 items-center justify-center p-8">
        <p className="text-muted-foreground">Account not found</p>
      </div>
    );
  }

  // In external view, agents tab is hidden — redirect to blueprints if it was active
  const resolvedTab: Tab = effectiveViewMode === "external" && activeTab === "agents" ? "blueprints" : activeTab;

  return (
    <PageContainer
      className="flex min-h-0"
      outerClassName="bg-background"
      outerChildren={
        <GradientGridWash
          colors={data.avatar_colors ?? undefined}
          opacity={0.4}
          className="absolute left-0 top-0 h-[700px] w-[calc((100%-min(100%,1500px))/2+20rem)] [mask-image:radial-gradient(ellipse_120%_150%_at_0%_0%,black_0%,transparent_80%)]"
        />
      }
    >
      <aside className="w-72 shrink-0 border-r border-border overflow-hidden">
        {editOpen && effectiveViewMode === "internal" ? (
          <ProfileEditSidebar data={data} onClose={() => setEditOpen(false)} />
        ) : (
          <ProfileViewSidebar
            data={data}
            isOwner={isOwner}
            blueprintCount={rawBlueprints.filter((bp) => effectiveViewMode === "external" ? bp.visibility === "public" : true).length}
            deploymentCount={rawDeployments.length}
            orgs={orgs}
            onEditOpen={effectiveViewMode === "internal" ? () => setEditOpen(true) : undefined}
          />
        )}
      </aside>

      <main className="relative flex flex-1 min-w-0 flex-col min-h-0">
        {/* View mode toggle */}
        {isOwner && (
          <div className="flex items-center justify-end px-8 pt-5 pb-0">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setViewMode((v) => v === "internal" ? "external" : "internal")}
              className="gap-1.5"
            >
              <Eye className="size-3.5" />
              {effectiveViewMode === "internal" ? "View as visitor" : "Back to owner view"}
            </Button>
          </div>
        )}

        {/* Tab bar */}
        <div className="flex items-end gap-5 px-8 pt-5 border-b border-border">
          <TabButton active={resolvedTab === "blueprints"} onClick={() => setActiveTab("blueprints")}>
            Blueprints
            {rawBlueprints.length > 0 && (
              <span className="ml-1.5 text-faint-foreground font-normal">
                {effectiveViewMode === "external"
                  ? rawBlueprints.filter((bp) => bp.visibility === "public").length
                  : rawBlueprints.length}
              </span>
            )}
          </TabButton>
          {effectiveViewMode === "internal" && (
            <TabButton active={resolvedTab === "agents"} onClick={() => setActiveTab("agents")}>
              Agents
              {rawDeployments.length > 0 && (
                <span className="ml-1.5 text-faint-foreground font-normal">{rawDeployments.length}</span>
              )}
            </TabButton>
          )}
          <TabButton active={resolvedTab === "hearts"} onClick={() => setActiveTab("hearts")}>
            Hearts
            {heartsData && heartsData.items.length > 0 && (
              <span className="ml-1.5 text-faint-foreground font-normal">
                {heartsData.items.length}{heartsData.next_cursor ? "+" : ""}
              </span>
            )}
          </TabButton>
        </div>

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-8 py-6">
          {resolvedTab === "blueprints" && (
            <BlueprintsTab
              blueprints={visibleBlueprints}
              accountName={data.name}
              isOwner={isOwner}
              isInternalView={effectiveViewMode === "internal"}
              search={bpSearch}
              onSearchChange={setBpSearch}
              visibility={bpVisibility}
              onVisibilityChange={setBpVisibility}
              sort={bpSort}
              onSortChange={setBpSort}
              reorderMode={bpReorderMode}
              hasCustomOrder={bpCustomOrder !== null}
              onEnterReorder={handleEnterReorder}
              onSaveReorder={handleSaveReorder}
            />
          )}
          {resolvedTab === "agents" && effectiveViewMode === "internal" && (
            <AgentsTab
              deployments={visibleDeployments}
              accountName={data.name}
              search={agentSearch}
              onSearchChange={setAgentSearch}
              sort={agentSort}
              onSortChange={setAgentSort}
            />
          )}
          {resolvedTab === "hearts" && (
            <HeartsTab
              accountName={data.name}
              isOwner={isOwner}
              search={heartSearch}
              onSearchChange={setHeartSearch}
              sort={heartSort}
              onSortChange={setHeartSort}
            />
          )}
        </div>
      </main>
    </PageContainer>
  );
}
