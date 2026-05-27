import { useState, useEffect } from "react";
import {
  DndContext,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  closestCenter,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  arrayMove,
  rectSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical, Check, Plus } from "lucide-react";
import { AgentMascots } from "@/components/AgentMascots";
import type { Blueprint } from "@/lib/api";
import { BlueprintCard } from "@/components/BlueprintCard";
import { getBlueprintDescription } from "@/lib/blueprint-utils";
import { Button } from "@/components/ui/button";
import { TabSearchInput, TabFilterDropdown } from "./TabToolbar";
import { EmptyState } from "@/components/EmptyState";
import { cn } from "@/lib/utils";

export type VisibilityFilter = "all" | "public" | "private";
export type BlueprintSort = "newest" | "name" | "deployed";
export type ReorderMode = "idle" | "editing" | "saved";

const VISIBILITY_OPTIONS: { value: VisibilityFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "public", label: "Public" },
  { value: "private", label: "Private" },
];

const SORT_OPTIONS: { value: BlueprintSort; label: string }[] = [
  { value: "newest", label: "Newest" },
  { value: "name", label: "Name A–Z" },
  { value: "deployed", label: "Most deployed" },
];

// ── Sortable card (inside DndContext) ─────────────────────────────────────────

function SortableBlueprintCard({
  id,
  agent,
  accountName,
  index,
}: {
  id: string;
  agent: Blueprint;
  accountName: string;
  index: number;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });

  // Freeze stagger delay at mount so reorders don't retrigger the entrance animation
  const [animDelay] = useState(() => Math.min(index * 45, 300));

  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className="h-full"
      {...attributes}
      {...listeners}
    >
      <div
        className={cn(
          "group relative h-full cursor-grab active:cursor-grabbing card-entering-edit",
          isDragging ? "opacity-50" : "card-draggable",
        )}
        style={{ animationDelay: `${animDelay}ms` }}
      >
        {/* Prevents accidental link navigation while dragging */}
        <div className="absolute inset-0 z-[3]" />
        <div className="pointer-events-none absolute right-2.5 top-2.5 z-[5] opacity-100 transition-opacity duration-150">
          <GripVertical className="h-3.5 w-3.5 text-foreground/30" />
        </div>
        <BlueprintCard
          slug={`${accountName}/${agent.name}`}
          account={accountName}
          name={agent.name}
          description={getBlueprintDescription(agent)}
          visibility={agent.visibility}
          avatarColors={agent.avatar_colors}
          deployCount={agent.metrics?.deploy_count}
          isDraft={agent.versions.length === 0}
        />
      </div>
    </div>
  );
}

// ── BlueprintsTab ─────────────────────────────────────────────────────────────

interface BlueprintsTabProps {
  blueprints: Blueprint[];
  accountName: string;
  canManage: boolean;
  isInternalView: boolean;
  search: string;
  onSearchChange: (v: string) => void;
  visibility: VisibilityFilter;
  onVisibilityChange: (v: VisibilityFilter) => void;
  sort: BlueprintSort;
  onSortChange: (v: BlueprintSort) => void;
  reorderMode: ReorderMode;
  onEnterReorder: () => void;
  onSaveReorder: (names: string[]) => void;
  isLoading?: boolean;
}

const CARD_GRID = "grid grid-cols-1 gap-3 @[540px]:grid-cols-2 @[900px]:grid-cols-3";

export function BlueprintsTab({
  blueprints,
  accountName,
  canManage,
  isInternalView,
  search,
  onSearchChange,
  visibility,
  onVisibilityChange,
  sort,
  onSortChange,
  reorderMode,
  onEnterReorder,
  onSaveReorder,
  isLoading,
}: BlueprintsTabProps) {
  const isEditing = reorderMode === "editing";
  const isSaved = reorderMode === "saved";

  const hasFilters = search.trim() !== "" || visibility !== "all" || sort !== "newest";
  const visibilityLabel = visibility === "all" ? "Visibility" : visibility === "public" ? "Public" : "Private";
  const sortLabel = SORT_OPTIONS.find((o) => o.value === sort)?.label ?? "Newest";

  // Local ordering state — seeded from `blueprints` on entering editing mode
  const [localOrder, setLocalOrder] = useState<Blueprint[]>(blueprints);
  useEffect(() => {
    if (isEditing) setLocalOrder(blueprints);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isEditing]); // intentionally only seed when mode changes to editing

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  function handleDragEnd({ active, over }: DragEndEvent) {
    if (!over || active.id === over.id) return;
    setLocalOrder((prev) => {
      const oldIndex = prev.findIndex((b) => b.name === active.id);
      const newIndex = prev.findIndex((b) => b.name === over.id);
      return arrayMove(prev, oldIndex, newIndex);
    });
  }

  return (
    <div className="flex flex-col gap-5">
      {/* Toolbar — hidden when there's nothing to filter/sort/reorder */}
      {(blueprints.length > 0 || hasFilters) && (
      <div className="flex flex-wrap items-center gap-x-3 gap-y-3">
        <TabSearchInput value={search} onChange={onSearchChange} placeholder="Search blueprints" disabled={isEditing} />

        {isInternalView && (
          <TabFilterDropdown
            value={visibility}
            onChange={onVisibilityChange}
            options={VISIBILITY_OPTIONS}
            triggerLabel={visibilityLabel}
            minWidth="min-w-32"
            disabled={isEditing}
          />
        )}

        <TabFilterDropdown
          value={sort}
          onChange={onSortChange}
          options={SORT_OPTIONS}
          triggerLabel={sortLabel}
          disabled={isEditing}
        />

        {canManage && isInternalView && blueprints.length > 0 && (
          isEditing ? (
            <Button size="sm" className="sm:ml-auto" onClick={() => onSaveReorder(localOrder.map((b) => b.name))}>
              Save changes
            </Button>
          ) : isSaved ? (
            <Button variant="ghost" size="sm" className="sm:ml-auto gap-1.5 text-success pointer-events-none">
              <Check className="size-3.5" />Saved
            </Button>
          ) : (
            <Button variant="outline" size="sm" onClick={onEnterReorder} className="sm:ml-auto">
              Customize order
            </Button>
          )
        )}
      </div>
      )}

      {isLoading && blueprints.length === 0 && !hasFilters ? null : blueprints.length === 0 ? (
        isInternalView ? (
          <EmptyState
            variant="card"
            icon={<AgentMascots size={36} />}
            title={hasFilters ? "No blueprints match your filters." : "No blueprints published yet."}
            {...(!hasFilters && canManage && {
              description: "Blueprints define what your agent does. Create one to get started.",
              actions: [{ label: "Create blueprint", to: "/new/custom", icon: <Plus className="size-4" /> }],
            })}
          />
        ) : (
          <p className="text-body text-muted-foreground">
            {hasFilters ? "No blueprints match your filters." : "No public blueprints yet."}
          </p>
        )
      ) : isEditing ? (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={localOrder.map((b) => b.name)} strategy={rectSortingStrategy}>
            <div className={CARD_GRID}>
              {localOrder.map((agent, i) => (
                <SortableBlueprintCard
                  key={agent.name}
                  id={agent.name}
                  agent={agent}
                  accountName={accountName}
                  index={i}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      ) : (
        <div className={CARD_GRID}>
          {blueprints.map((agent) => (
            <div key={agent.name} className="relative h-full">
              <BlueprintCard
                slug={`${accountName}/${agent.name}`}
                account={accountName}
                name={agent.name}
                description={getBlueprintDescription(agent)}
                visibility={agent.visibility}
                avatarColors={agent.avatar_colors}
                deployCount={agent.metrics?.deploy_count}
                isDraft={agent.versions.length === 0}
                onArchive={canManage && isInternalView ? () => {} : undefined}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
