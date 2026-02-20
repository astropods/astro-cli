import { useState } from "react";
import { Link } from "react-router";
import { Sparkles, Loader2 } from "lucide-react";

type WizardStep = "area" | "data" | "loading" | "results";

interface WizardProps {
  onClose: () => void;
}

const businessAreas = [
  "Engineering",
  "Ops & Incidents",
  "Team Communication",
  "Reporting & Insights",
];

const dataSourcesByArea: Record<string, string[]> = {
  Engineering: ["GitHub", "Jira", "Linear", "GitLab"],
  "Ops & Incidents": ["PagerDuty", "Slack", "Opsgenie", "ServiceNow"],
  "Team Communication": ["Slack", "Microsoft Teams", "Discord", "Email"],
  "Reporting & Insights": ["Jira", "Salesforce", "Zendesk", "Notion"],
};

const recommendationsByArea: Record<
  string,
  Array<{
    slug: string;
    name: string;
    tags: string[];
    description: string;
  }>
> = {
  Engineering: [
    {
      slug: "sprint-insight-engine",
      name: "Sprint Insight Engine",
      tags: ["Jira", "Slack"],
      description: "Tracks sprint progress and generates automated status reports.",
    },
    {
      slug: "security-monitor",
      name: "Security Monitor",
      tags: ["GitHub", "Slack"],
      description: "Continuously scans for vulnerabilities and alerts your security team.",
    },
  ],
  "Ops & Incidents": [
    {
      slug: "incident-command",
      name: "Incident Command",
      tags: ["PagerDuty", "Slack"],
      description: "Automatically routes and escalates incidents based on severity.",
    },
  ],
  "Team Communication": [
    {
      slug: "personalized-support-responses",
      name: "Personalized Support Responses",
      tags: ["Zendesk", "Slack"],
      description: "Drafts personalized support replies using customer context.",
    },
  ],
  "Reporting & Insights": [
    {
      slug: "customer-insight-engine",
      name: "Customer Insight Engine",
      tags: ["Zendesk", "Slack"],
      description: "Analyzes customer feedback to surface actionable insights.",
    },
    {
      slug: "product-research-intel",
      name: "Product Research Intel",
      tags: ["Slack", "Notion"],
      description: "Aggregates product research and competitive intelligence.",
    },
  ],
};

export function FindAgentsWizard({ onClose }: WizardProps) {
  const [step, setStep] = useState<WizardStep>("area");
  const [selectedArea, setSelectedArea] = useState<string>("");
  const [selectedDataSources, setSelectedDataSources] = useState<string[]>([]);

  const handleAreaSelect = (area: string) => {
    setSelectedArea(area);
    setStep("data");
  };

  const handleDataSourceSelect = (source: string) => {
    const newSources = selectedDataSources.includes(source)
      ? selectedDataSources.filter((s) => s !== source)
      : [...selectedDataSources, source];
    setSelectedDataSources(newSources);
  };

  const handleContinue = () => {
    setStep("loading");
    setTimeout(() => setStep("results"), 1500);
  };

  const recommendations = selectedArea
    ? recommendationsByArea[selectedArea] || []
    : [];

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="flex flex-col gap-3">
        {/* Assistant intro */}
        <div className="flex gap-2.5 max-w-[90%] self-start">
          <div className="w-7 h-7 border border-stone-300 flex items-center justify-center shrink-0">
            <Sparkles size={14} />
          </div>
          <div className="border border-stone-300 p-3 text-sm">
            <p>
              Hi! I'm here to help you find the perfect agents for your needs.
              Let's start with a few questions.
            </p>
          </div>
        </div>

        {/* Business area question */}
        <div className="flex gap-2.5 max-w-[90%] self-start">
          <div className="w-7 h-7 border border-stone-300 flex items-center justify-center shrink-0">
            <Sparkles size={14} />
          </div>
          <div className="border border-stone-300 p-3 text-sm">
            <p className="mb-2.5">Which business area would you like to focus on?</p>
            <div className="flex flex-wrap gap-1.5">
              {businessAreas.map((area) => (
                <button
                  key={area}
                  className={`px-3 py-1.5 border text-sm cursor-pointer ${
                    selectedArea === area
                      ? "bg-stone-800 text-white border-stone-800"
                      : "bg-white border-stone-300 hover:bg-stone-50"
                  }`}
                  onClick={() => handleAreaSelect(area)}
                >
                  {area}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* User selection */}
        {selectedArea && (
          <div className="flex gap-2.5 max-w-[90%] self-end flex-row-reverse">
            <div className="border border-stone-300 p-3 text-sm bg-stone-100">
              {selectedArea}
            </div>
          </div>
        )}

        {/* Data sources question */}
        {step !== "area" && (
          <div className="flex gap-2.5 max-w-[90%] self-start">
            <div className="w-7 h-7 border border-stone-300 flex items-center justify-center shrink-0">
              <Sparkles size={14} />
            </div>
            <div className="border border-stone-300 p-3 text-sm">
              <p className="mb-2.5">Great choice! Where does your data live?</p>
              <div className="flex flex-wrap gap-1.5">
                {dataSourcesByArea[selectedArea]?.map((source) => (
                  <button
                    key={source}
                    className={`px-3 py-1.5 border text-sm cursor-pointer ${
                      selectedDataSources.includes(source)
                        ? "bg-stone-800 text-white border-stone-800"
                        : "bg-white border-stone-300 hover:bg-stone-50"
                    }`}
                    onClick={() => handleDataSourceSelect(source)}
                  >
                    {source}
                  </button>
                ))}
              </div>
              {selectedDataSources.length > 0 && step === "data" && (
                <button
                  className="mt-3 px-4 py-2 bg-stone-800 text-white border border-stone-800 text-sm cursor-pointer"
                  onClick={handleContinue}
                >
                  Continue
                </button>
              )}
            </div>
          </div>
        )}

        {/* Loading state */}
        {step === "loading" && (
          <div className="flex gap-2.5 max-w-[90%] self-start">
            <div className="w-7 h-7 border border-stone-300 flex items-center justify-center shrink-0">
              <Sparkles size={14} />
            </div>
            <div className="border border-stone-300 p-3 text-sm flex items-center gap-2.5 text-stone-600">
              <Loader2 className="animate-spin" size={18} />
              <span>Gathering your recs...</span>
            </div>
          </div>
        )}

        {/* Results */}
        {step === "results" && (
          <div className="flex gap-2.5 max-w-[90%] self-start">
            <div className="w-7 h-7 border border-stone-300 flex items-center justify-center shrink-0">
              <Sparkles size={14} />
            </div>
            <div className="border border-stone-300 p-3 text-sm">
              <p className="mb-2.5">Based on your answers, here are my recommendations:</p>
              <div className="flex flex-col gap-2.5 mt-2">
                {recommendations.map((agent) => (
                  <div key={agent.slug} className="border border-stone-300 p-3">
                    <h4 className="font-semibold text-sm mb-1.5">{agent.name}</h4>
                    <div className="flex gap-1 mb-1.5">
                      {agent.tags.map((tag) => (
                        <span
                          key={tag}
                          className="px-1.5 py-0.5 text-xs border border-stone-300 bg-stone-100"
                        >
                          {tag}
                        </span>
                      ))}
                    </div>
                    <p className="text-stone-600 text-sm mb-2">{agent.description}</p>
                    <Link
                      to={`/${agent.slug}`}
                      className="inline-block px-3 py-1.5 border border-stone-300 text-sm text-stone-700 no-underline hover:bg-stone-50"
                      onClick={onClose}
                    >
                      View details
                    </Link>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
