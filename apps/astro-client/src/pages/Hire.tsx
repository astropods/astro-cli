import { useState, useMemo } from "react";
import { Link, useSearchParams, useOutletContext } from "react-router-dom";
import {
  PaperAirplaneIcon,
  ChevronDownIcon,
  MagnifyingGlassIcon,
} from "@heroicons/react/24/outline";
import { X } from "lucide-react";
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

const categories = [
  "All",
  "Developer Tools",
  "IT Support",
  "Customer Support",
  "Analytics",
  "Security",
];

const sortOptions = [
  { label: "Most recent", value: "recent" },
  { label: "Name A-Z", value: "name-asc" },
] as const;

type SortValue = (typeof sortOptions)[number]["value"];

const agents = [
  {
    slug: "customer-insight-engine",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    integrations: ["Zendesk", "Slack", "Intercom", "Salesforce"],
    categories: ["Analytics"],
  },
  {
    slug: "incident-command",
    name: "Incident Command",
    description:
      "Automatically routes and escalates incidents based on severity and team availability.",
    integrations: ["PagerDuty", "Slack", "Opsgenie", "ServiceNow"],
    categories: ["IT Support"],
  },
  {
    slug: "personalized-support-responses",
    name: "Personalized Support Responses",
    description:
      "Drafts personalized support replies using customer context and history.",
    integrations: ["Zendesk", "Intercom", "Freshdesk", "Slack"],
    categories: ["Customer Support"],
  },
  {
    slug: "security-monitor",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    integrations: ["GitHub", "GitLab", "Slack", "Jira"],
    categories: ["Security"],
  },
  {
    slug: "sprint-insight-engine",
    name: "Sprint Insight Engine",
    description:
      "Tracks sprint progress and generates automated status reports.",
    integrations: ["Jira", "Linear", "Asana", "Slack"],
    categories: ["Developer Tools"],
  },
  {
    slug: "product-research-intel",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    integrations: ["Slack", "Notion", "Confluence", "Google Docs"],
    categories: ["Analytics"],
  },
];

export function Hire() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [sortBy, setSortBy] = useState<SortValue>("recent");
  const { openAuthModal } = useOutletContext<LayoutContext>();

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
        agent.categories.includes(selectedCategory);
      return matchesSearch && matchesCategory;
    });

    if (sortBy === "name-asc") {
      filtered.sort((a, b) => a.name.localeCompare(b.name));
    }

    return filtered;
  }, [searchQuery, selectedCategory, sortBy]);

  const currentSort =
    sortOptions.find((o) => o.value === sortBy)?.label ?? "Sort";

  return (
    <div className="@container w-full flex-1 px-6 pb-6 pt-4 md:px-8 md:pb-8 md:pt-6 max-w-[1500px] mx-auto">
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

      {/* Agent card grid */}
      <div className="grid grid-cols-1 gap-6 @[540px]:grid-cols-2 @[820px]:grid-cols-3 @[1100px]:grid-cols-4">
        {filteredAgents.map((agent) => (
          <AgentCard
            key={agent.slug}
            slug={agent.slug}
            name={agent.name}
            description={agent.description}
            integrations={agent.integrations}
            categories={agent.categories}
            onInstall={() => openAuthModal()}
          />
        ))}
      </div>

      {/* Wizard overlay — unchanged */}
      {isWizardOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="relative flex max-h-[80vh] w-full max-w-[500px] flex-col overflow-hidden border border-gray-300 bg-white">
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
