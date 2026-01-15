import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Search, X } from "lucide-react";
import { FindAgentsWizard } from "../components/FindAgentsWizard";

const categories = [
  "All",
  "Developer Tools",
  "IT Support",
  "Customer Support",
  "Analytics",
  "Security",
];

const agents = [
  {
    slug: "customer-insight-engine",
    name: "Customer Insight Engine",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    tags: ["Zendesk", "Slack"],
    category: "Analytics",
  },
  {
    slug: "incident-command",
    name: "Incident Command",
    description:
      "Automatically routes and escalates incidents based on severity and team availability.",
    tags: ["PagerDuty", "Slack"],
    category: "IT Support",
  },
  {
    slug: "personalized-support-responses",
    name: "Personalized Support Responses",
    description:
      "Drafts personalized support replies using customer context and history.",
    tags: ["Zendesk", "Slack"],
    category: "Customer Support",
  },
  {
    slug: "security-monitor",
    name: "Security Monitor",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    tags: ["GitHub", "Slack"],
    category: "Security",
  },
  {
    slug: "sprint-insight-engine",
    name: "Sprint Insight Engine",
    description:
      "Tracks sprint progress and generates automated status reports.",
    tags: ["Jira", "Slack"],
    category: "Developer Tools",
  },
  {
    slug: "product-research-intel",
    name: "Product Research Intel",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    tags: ["Slack", "Notion"],
    category: "Analytics",
  },
];

export function Hire() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");

  const isWizardOpen = searchParams.get("start") === "true";

  const closeWizard = () => {
    setSearchParams({});
  };

  const filteredAgents = agents.filter((agent) => {
    const matchesSearch = agent.name
      .toLowerCase()
      .includes(searchQuery.toLowerCase());
    const matchesCategory =
      selectedCategory === "All" || agent.category === selectedCategory;
    return matchesSearch && matchesCategory;
  });

  return (
    <div className="max-w-[1000px]">
      <div className="flex justify-between items-start mb-6">
        <div>
          <h1 className="text-2xl font-semibold mb-1">Agents</h1>
          <p className="text-gray-600 text-sm">
            Discover pre-built agents to automate your workflows
          </p>
        </div>
        <Link
          to="/request-agent"
          className="px-4 py-2 border border-gray-300 bg-gray-100 text-sm text-gray-700 no-underline hover:bg-gray-200"
        >
          Request an agent
        </Link>
      </div>

      <div className="flex flex-col gap-3 mb-6">
        <div className="relative max-w-[300px]">
          <Search
            size={16}
            className="absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-500"
          />
          <input
            type="text"
            placeholder="Search agents..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full py-2 px-3 pl-8 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
          />
        </div>

        <div className="flex flex-wrap gap-2">
          {categories.map((category) => (
            <button
              key={category}
              className={`px-3 py-1.5 border text-sm cursor-pointer ${
                selectedCategory === category
                  ? "bg-gray-800 text-white border-gray-800"
                  : "bg-white border-gray-300 hover:bg-gray-50"
              }`}
              onClick={() => setSelectedCategory(category)}
            >
              {category}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filteredAgents.map((agent) => (
          <div
            key={agent.slug}
            className="border border-gray-300 p-4 flex flex-col"
          >
            <h3 className="font-semibold mb-2">{agent.name}</h3>
            <p className="text-gray-600 text-sm mb-3 flex-1">
              {agent.description}
            </p>
            <div className="flex flex-wrap gap-1.5 mb-3">
              {agent.tags.map((tag) => (
                <span
                  key={tag}
                  className="px-2 py-0.5 text-xs border border-gray-300 bg-gray-100"
                >
                  {tag}
                </span>
              ))}
            </div>
            <Link
              to={`/hire/${agent.slug}`}
              className="px-4 py-2 border border-gray-300 text-sm text-center text-gray-700 no-underline hover:bg-gray-50"
            >
              View details
            </Link>
          </div>
        ))}
      </div>

      {isWizardOpen && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white border border-gray-300 w-full max-w-[500px] max-h-[80vh] relative overflow-hidden flex flex-col">
            <button
              className="absolute top-3 right-3 bg-transparent border-none cursor-pointer z-10"
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
