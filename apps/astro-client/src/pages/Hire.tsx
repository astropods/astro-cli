import { useState, useMemo } from "react";
import { Link, useSearchParams, useOutletContext } from "react-router-dom";
import {
  PaperAirplaneIcon,
  ChevronDownIcon,
  MagnifyingGlassIcon,
} from "@heroicons/react/24/outline";
import { Loader2, X } from "lucide-react";
import { PageTitle } from "@/components/PageTitle";
import { AgentCard } from "@/components/AgentCard";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { FindAgentsWizard } from "../components/FindAgentsWizard";
import type { LayoutContext } from "@/components/Layout";
import { useAgents } from "@/api/queries";
import type { Agent } from "@/lib/api";

const sortOptions = [
  { label: "Most recent", value: "recent" },
  { label: "Name A-Z", value: "name-asc" },
] as const;

type SortValue = (typeof sortOptions)[number]["value"];

function getLatestVersion(agent: Agent) {
  return agent.versions[0];
}

function getLatestSpec(agent: Agent) {
  return getLatestVersion(agent)?.spec;
}

function getAgentCategories(agent: Agent): string[] {
  return getLatestSpec(agent)?.meta?.tags ?? [];
}

function getAgentDescription(agent: Agent): string {
  return getLatestSpec(agent)?.meta?.description ?? agent.name;
}

function getAgentIntegrations(agent: Agent): string[] {
  const tools = getLatestSpec(agent)?.integrations?.tools;
  if (!tools) return [];
  return [...new Set(tools.map((t) => t.provider))];
}

export function Hire() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [sortBy, setSortBy] = useState<SortValue>("recent");
  const { openAuthModal } = useOutletContext<LayoutContext>();

  const { data, isLoading, isError, error, refetch } = useAgents();
  const agents = data?.agents ?? [];

  const categories = useMemo(() => {
    const tagSet = new Set<string>();
    for (const agent of agents) {
      for (const tag of getAgentCategories(agent)) {
        tagSet.add(tag);
      }
    }
    return ["All", ...Array.from(tagSet).sort()];
  }, [agents]);

  const isWizardOpen = searchParams.get("start") === "true";

  const closeWizard = () => {
    setSearchParams({});
  };

  const filteredAgents = useMemo(() => {
    const filtered = agents.filter((agent) => {
      const matchesSearch = agent.name
        .toLowerCase()
        .includes(searchQuery.toLowerCase());
      const matchesCategory =
        selectedCategory === "All" ||
        getAgentCategories(agent).includes(selectedCategory);
      return matchesSearch && matchesCategory;
    });

    if (sortBy === "recent") {
      filtered.sort((a, b) => {
        const aTime = getLatestVersion(a)?.published_at ?? "";
        const bTime = getLatestVersion(b)?.published_at ?? "";
        return bTime.localeCompare(aTime);
      });
    } else if (sortBy === "name-asc") {
      filtered.sort((a, b) => a.name.localeCompare(b.name));
    }

    return filtered;
  }, [agents, searchQuery, selectedCategory, sortBy]);

  const currentSort =
    sortOptions.find((o) => o.value === sortBy)?.label ?? "Sort";

  return (
    <div className="@container w-full flex-1 overflow-y-auto px-6 pb-6 pt-4 md:px-8 md:pb-8 md:pt-6 max-w-[1500px] mx-auto">
      <PageTitle
        title="Available Agents"
        subtitle="Browse agents available within your organization"
        actions={
          <Button asChild className="hidden @[540px]:inline-flex">
            <Link to="/request-agent">
              <PaperAirplaneIcon className="size-4" />
              Request agent
            </Link>
          </Button>
        }
        className="mb-6"
      />

      {/* Search + filter bar */}
      <div className="mb-6 flex flex-col @[540px]:flex-row @[540px]:items-center gap-3">
        <div className="relative flex-1 max-w-[648px]">
          <MagnifyingGlassIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search or describe what you're looking for"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9"
          />
        </div>

        <div className="flex items-center gap-3 @[540px]:ml-auto">
          {/* Category filter */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">
                {selectedCategory === "All" ? "Industry" : selectedCategory}
                <ChevronDownIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {categories.map((category) => (
                <DropdownMenuItem
                  key={category}
                  onSelect={() => setSelectedCategory(category)}
                >
                  {category}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          {/* Sort */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline">
                {currentSort}
                <ChevronDownIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {sortOptions.map((option) => (
                <DropdownMenuItem
                  key={option.value}
                  onSelect={() => setSortBy(option.value)}
                >
                  {option.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Loading state */}
      {isLoading ? (
        <div role="status" aria-label="Loading agents" className="flex items-center justify-center py-12">
          <Loader2 size={32} className="animate-spin text-stone-500" />
        </div>
      ) : isError ? (
        <div className="p-4 bg-red-50 border border-red-200 text-red-700">
          <p className="font-medium">Failed to load agents</p>
          <p className="text-sm">
            {(error as { error_description?: string })?.error_description ??
              (error instanceof Error ? error.message : "An unexpected error occurred")}
          </p>
          <button
            onClick={() => refetch()}
            className="mt-2 px-3 py-1 text-sm border border-red-300 bg-white text-red-700 hover:bg-red-50 cursor-pointer"
          >
            Retry
          </button>
        </div>
      ) : agents.length === 0 ? (
        <div className="p-8 border border-stone-300 text-center">
          <h3 className="text-lg font-medium mb-2">No agents available</h3>
          <p className="text-stone-600 text-sm">
            There are no agents in the registry yet.
          </p>
        </div>
      ) : (
        /* Agent card grid */
        <div className="grid grid-cols-1 gap-6 @[540px]:grid-cols-2 @[820px]:grid-cols-3 @[1100px]:grid-cols-4">
          {filteredAgents.map((agent) => (
            <AgentCard
              key={agent.name}
              slug={agent.name}
              name={agent.name}
              description={getAgentDescription(agent)}
              integrations={getAgentIntegrations(agent)}
              categories={getAgentCategories(agent)}
              onInstall={() => openAuthModal()}
            />
          ))}
        </div>
      )}

      {/* Wizard overlay — unchanged */}
      {isWizardOpen && (
        <div role="dialog" aria-label="Find agents wizard" className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="relative flex max-h-[80vh] w-full max-w-[500px] flex-col overflow-hidden border border-stone-300 bg-white">
            <button
              className="absolute right-3 top-3 z-10 cursor-pointer border-none bg-transparent"
              onClick={closeWizard}
            >
              <X size={20} />
            </button>
            <FindAgentsWizard onClose={closeWizard} />
          </div>
        </div>
      )}
    </div>
  );
}
