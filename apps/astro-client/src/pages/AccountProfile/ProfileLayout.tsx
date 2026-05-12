import { useState, useMemo, useEffect, useCallback, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import type { Blueprint, AgentDeployment, AccountPublic } from "@/lib/api";
import { useUpdateAccountProfile } from "@/api/queries/accounts";
import { PageContainer } from "@/components/PageLayout";
import { GradientGridWash } from "@/components/GradientGridWash";
import { Button } from "@/components/ui/button";
import { ArrowUpRight } from "lucide-react";
import { BlueprintsTab } from "./BlueprintsTab";
import { AgentsTab } from "./AgentsTab";
import { HeartsTab } from "./HeartsTab";
import { TabButton } from "./TabToolbar";
import type { VisibilityFilter, BlueprintSort, ReorderMode } from "./BlueprintsTab";
import type { AgentSort } from "./AgentsTab";
import type { HeartSort } from "./HeartsTab";

function latestPublished(bp: Blueprint): string {
  return bp.versions.reduce((m, v) => (v.published_at > m ? v.published_at : m), "");
}

// ── Types ─────────────────────────────────────────────────────────────────────

type Tab = "blueprints" | "agents" | "hearts";

export interface SidebarRenderOpts {
  blueprintCount: number;
  deploymentCount: number;
  onEditOpen: (() => void) | undefined;
  isBlueprintsLoading: boolean;
  isDeploymentsLoading: boolean;
}

export interface EditSidebarRenderOpts {
  onClose: () => void;
}

export interface HeartsConfig {
  isOwner: boolean;
  search: string;
  onSearchChange: (v: string) => void;
  sort: HeartSort;
  onSortChange: (v: HeartSort) => void;
  tabCount?: string | null;
}

export interface ProfileLayoutProps {
  data: AccountPublic;
  isAdmin: boolean;
  canViewDeployments: boolean;
  rawBlueprints: Blueprint[];
  rawDeployments: AgentDeployment[];
  isBlueprintsLoading?: boolean;
  isDeploymentsLoading?: boolean;
  renderViewSidebar: (opts: SidebarRenderOpts) => ReactNode;
  renderEditSidebar: (opts: EditSidebarRenderOpts) => ReactNode;
  hearts?: HeartsConfig;
}

// ── ProfileLayout ─────────────────────────────────────────────────────────────

export function ProfileLayout({
  data,
  isAdmin,
  canViewDeployments,
  rawBlueprints,
  rawDeployments,
  isBlueprintsLoading = false,
  isDeploymentsLoading = false,
  renderViewSidebar,
  renderEditSidebar,
  hearts,
}: ProfileLayoutProps) {
  // ── Visitor mode — force public view even when owner ─────────────────────
  const [searchParams] = useSearchParams();
  const isVisitorMode = searchParams.has("visitor");
  const isAdminView = isAdmin && !isVisitorMode;    // edit profile, view-as-visitor
  const isInternalView = canViewDeployments && !isVisitorMode; // private blueprints, visibility filter, agents

  // ── Sidebar edit ──────────────────────────────────────────────────────────
  const [editOpen, setEditOpen] = useState(false);

  // ── Tab ───────────────────────────────────────────────────────────────────
  const [activeTab, setActiveTab] = useState<Tab>("blueprints");

  // ── Blueprint filters ─────────────────────────────────────────────────────
  const [bpSearch, setBpSearch] = useState("");
  const [bpVisibility, setBpVisibility] = useState<VisibilityFilter>("all");
  const [bpSort, setBpSort] = useState<BlueprintSort>("newest");

  // ── Blueprint reorder ─────────────────────────────────────────────────────
  const updateAccountProfile = useUpdateAccountProfile();
  const [bpReorderMode, setBpReorderMode] = useState<ReorderMode>("idle");
  const [bpCustomOrder, setBpCustomOrder] = useState<string[] | null>(null);

  useEffect(() => {
    if (data.blueprint_order) setBpCustomOrder(data.blueprint_order);
  }, [data.blueprint_order]);

  const handleEnterReorder = useCallback(() => {
    setBpSearch("");
    setBpVisibility("all");
    setBpSort("newest");
    setBpReorderMode("editing");
  }, []);

  const handleSaveReorder = useCallback(
    (names: string[]) => {
      const prevOrder = bpCustomOrder;
      setBpCustomOrder(names);
      setBpReorderMode("saved");
      updateAccountProfile.mutate(
        { account: data.name, blueprint_order: names },
        {
          onSuccess: () => setTimeout(() => setBpReorderMode("idle"), 1500),
          onError: () => {
            setBpCustomOrder(prevOrder);
            setBpReorderMode("editing");
          },
        },
      );
    },
    [data.name, bpCustomOrder, updateAccountProfile],
  );

  // ── Agent filters ─────────────────────────────────────────────────────────
  const [agentSearch, setAgentSearch] = useState("");
  const [agentSort, setAgentSort] = useState<AgentSort>("modified");

  // ── Blueprint filtering + sorting ─────────────────────────────────────────
  const visibleBlueprints = useMemo(() => {
    let list = isInternalView
      ? rawBlueprints
      : rawBlueprints.filter((bp) => bp.visibility === "public");

    if (isInternalView && bpVisibility !== "all") {
      list = list.filter((bp) => bp.visibility === bpVisibility);
    }

    if (bpSearch.trim()) {
      const q = bpSearch.toLowerCase();
      list = list.filter((bp) => bp.name.toLowerCase().includes(q));
    }

    if (bpSort === "newest" && bpCustomOrder) {
      const orderMap = new Map(bpCustomOrder.map((name, i) => [name, i]));
      list = [...list].sort((a, b) => {
        const ai = orderMap.get(a.name) ?? Infinity;
        const bi = orderMap.get(b.name) ?? Infinity;
        if (ai !== bi) return ai - bi;
        return latestPublished(b).localeCompare(latestPublished(a));
      });
    } else if (bpSort === "newest") {
      list = [...list].sort((a, b) => latestPublished(b).localeCompare(latestPublished(a)));
    } else if (bpSort === "name") {
      list = [...list].sort((a, b) => a.name.localeCompare(b.name));
    } else if (bpSort === "deployed") {
      list = [...list].sort(
        (a, b) => (b.metrics?.deploy_count ?? 0) - (a.metrics?.deploy_count ?? 0),
      );
    }

    return list;
  }, [rawBlueprints, isInternalView, bpVisibility, bpSearch, bpSort, bpCustomOrder]);

  // ── Agent filtering + sorting ─────────────────────────────────────────────
  const visibleDeployments = useMemo(() => {
    let list = rawDeployments;

    if (agentSearch.trim()) {
      const q = agentSearch.toLowerCase();
      list = list.filter(
        (d) =>
          d.name.toLowerCase().includes(q) || (d.display_name ?? "").toLowerCase().includes(q),
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

  const canSeeAgentsTab = canViewDeployments && !isVisitorMode;

  // Agents tab hidden for non-members and in visitor mode; fall back to blueprints
  const resolvedTab: Tab =
    !canSeeAgentsTab && activeTab === "agents" ? "blueprints" : activeTab;

  const publicBlueprintCount = rawBlueprints.filter((bp) => bp.visibility === "public").length;
  const blueprintCount = isInternalView ? rawBlueprints.length : publicBlueprintCount;

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
        {editOpen
          ? renderEditSidebar({ onClose: () => setEditOpen(false) })
          : renderViewSidebar({
              blueprintCount,
              deploymentCount: rawDeployments.length,
              onEditOpen: isAdminView ? () => setEditOpen(true) : undefined,
              isBlueprintsLoading,
              isDeploymentsLoading,
            })}
      </aside>

      <main className="relative flex flex-1 min-w-0 flex-col min-h-0">
        {/* Tab bar */}
        <div className="flex items-end gap-5 px-8 pt-5 border-b border-border">
          <TabButton
            active={resolvedTab === "blueprints"}
            onClick={() => setActiveTab("blueprints")}
          >
            Blueprints
            {rawBlueprints.length > 0 && (
              <span className="ml-1.5 text-faint-foreground font-normal">{blueprintCount}</span>
            )}
          </TabButton>

          {canSeeAgentsTab && (
            <TabButton active={resolvedTab === "agents"} onClick={() => setActiveTab("agents")}>
              Agents
              {rawDeployments.length > 0 && (
                <span className="ml-1.5 text-faint-foreground font-normal">
                  {rawDeployments.length}
                </span>
              )}
            </TabButton>
          )}

          {hearts && (
            <TabButton active={resolvedTab === "hearts"} onClick={() => setActiveTab("hearts")}>
              Hearts
              {hearts.tabCount && (
                <span className="ml-1.5 text-faint-foreground font-normal">{hearts.tabCount}</span>
              )}
            </TabButton>
          )}

          {isAdminView && (
            <Button
              variant="ghost"
              size="sm"
              asChild
              className="gap-1.5 mb-2 ml-auto order-last shrink-0"
            >
              <Link to={`/${data.name}?visitor`} target="_blank" rel="noopener noreferrer">
                View as visitor
                <ArrowUpRight className="size-3" />
              </Link>
            </Button>
          )}
        </div>

        {/* Tab content */}
        <div className="flex-1 overflow-y-auto px-8 py-6">
          {resolvedTab === "blueprints" && (
            <BlueprintsTab
              blueprints={visibleBlueprints}
              accountName={data.name}
              displayName={data.display_name || data.name}
              canManage={isAdmin}
              isInternalView={isInternalView}
              search={bpSearch}
              onSearchChange={setBpSearch}
              visibility={bpVisibility}
              onVisibilityChange={setBpVisibility}
              sort={bpSort}
              onSortChange={setBpSort}
              reorderMode={bpReorderMode}
              onEnterReorder={handleEnterReorder}
              onSaveReorder={handleSaveReorder}
              isLoading={isBlueprintsLoading}
              skeletonCount={data.blueprint_order?.length || 3}
            />
          )}
          {resolvedTab === "agents" && canSeeAgentsTab && (
            <AgentsTab
              deployments={visibleDeployments}
              accountName={data.name}
              search={agentSearch}
              onSearchChange={setAgentSearch}
              sort={agentSort}
              onSortChange={setAgentSort}
              isLoading={isDeploymentsLoading}
            />
          )}
          {resolvedTab === "hearts" && hearts && (
            <HeartsTab
              accountName={data.name}
              isOwner={hearts.isOwner}
              search={hearts.search}
              onSearchChange={hearts.onSearchChange}
              sort={hearts.sort}
              onSortChange={hearts.onSortChange}
            />
          )}
        </div>
      </main>
    </PageContainer>
  );
}
