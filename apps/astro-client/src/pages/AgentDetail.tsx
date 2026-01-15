import { useParams, useOutletContext, Link } from "react-router-dom";
import { ArrowLeft } from "lucide-react";

interface OutletContext {
  openAuthModal: () => void;
}

const agentsData: Record<
  string,
  {
    name: string;
    category: string;
    description: string;
    longDescription: string;
    tags: string[];
    integrations: string[];
    howItWorks: string[];
    safetyPermissions: string[];
    suggestedPrompts: string[];
  }
> = {
  "customer-insight-engine": {
    name: "Customer Insight Engine",
    category: "Analytics",
    description:
      "Analyzes customer feedback to surface actionable insights and trends.",
    longDescription:
      "The Customer Insight Engine automatically aggregates feedback from your support channels, identifies patterns, and surfaces the most impactful insights for your product and support teams. It uses advanced NLP to categorize feedback, detect sentiment, and prioritize issues based on customer impact.",
    tags: ["Analytics", "Zendesk", "Slack"],
    integrations: ["Zendesk", "Slack", "Intercom", "Salesforce"],
    howItWorks: [
      "Connects to your support channels and pulls customer feedback",
      "Analyzes feedback using NLP to identify themes and sentiment",
      "Generates weekly insight reports with actionable recommendations",
      "Posts summaries to your designated Slack channel",
    ],
    safetyPermissions: [
      "Read-only access to support tickets",
      "Posts to designated Slack channel only",
      "No access to customer PII beyond support context",
    ],
    suggestedPrompts: [
      "Show top customer pain points",
      "Find feature requests",
      "Analyze churn feedback",
    ],
  },
  "incident-command": {
    name: "Incident Command",
    category: "IT Support",
    description:
      "Automatically routes and escalates incidents based on severity and team availability.",
    longDescription:
      "Incident Command streamlines your incident response by intelligently routing alerts to the right team members based on severity, on-call schedules, and team expertise. It reduces response times and ensures critical issues never fall through the cracks.",
    tags: ["IT Support", "PagerDuty", "Slack"],
    integrations: ["PagerDuty", "Slack", "Opsgenie", "ServiceNow"],
    howItWorks: [
      "Monitors incoming alerts from your alerting tools",
      "Assesses severity and categorizes incidents",
      "Routes to appropriate on-call personnel",
      "Escalates unacknowledged incidents automatically",
    ],
    safetyPermissions: [
      "Read access to alert data",
      "Can create and update incidents",
      "Posts to designated incident channels",
    ],
    suggestedPrompts: [
      "Show current incidents",
      "Who is on-call?",
      "Escalate this incident",
    ],
  },
  "personalized-support-responses": {
    name: "Personalized Support Responses",
    category: "Customer Support",
    description:
      "Drafts personalized support replies using customer context and history.",
    longDescription:
      "This agent helps your support team respond faster by drafting personalized replies that consider the customer's history, previous interactions, and the specific context of their issue. Agents can review and send with a single click.",
    tags: ["Customer Support", "Zendesk", "Slack"],
    integrations: ["Zendesk", "Intercom", "Freshdesk", "Slack"],
    howItWorks: [
      "Reads incoming support ticket and customer history",
      "Analyzes similar resolved tickets for best practices",
      "Drafts a personalized response for agent review",
      "Learns from agent edits to improve future drafts",
    ],
    safetyPermissions: [
      "Read access to support tickets and customer history",
      "Drafts only - human approval required to send",
      "No automated customer communication",
    ],
    suggestedPrompts: [
      "Draft a reply for this ticket",
      "Find similar resolved tickets",
      "Suggest a resolution",
    ],
  },
  "security-monitor": {
    name: "Security Monitor",
    category: "Security",
    description:
      "Continuously scans for vulnerabilities and alerts your security team.",
    longDescription:
      "Security Monitor provides continuous vulnerability scanning across your repositories and infrastructure. It identifies security issues, prioritizes them by severity, and alerts your security team with actionable remediation steps.",
    tags: ["Security", "GitHub", "Slack"],
    integrations: ["GitHub", "GitLab", "Slack", "Jira"],
    howItWorks: [
      "Scans repositories for known vulnerabilities",
      "Monitors dependencies for security advisories",
      "Prioritizes findings by severity and exploitability",
      "Alerts security team with remediation guidance",
    ],
    safetyPermissions: [
      "Read-only access to repository code",
      "Cannot modify code or merge changes",
      "Alerts sent to designated security channel",
    ],
    suggestedPrompts: [
      "Show critical vulnerabilities",
      "Scan this repository",
      "What needs immediate attention?",
    ],
  },
  "sprint-insight-engine": {
    name: "Sprint Insight Engine",
    category: "Developer Tools",
    description:
      "Tracks sprint progress and generates automated status reports.",
    longDescription:
      "Sprint Insight Engine keeps your team informed by automatically tracking sprint progress, identifying blockers, and generating status reports. It integrates with your project management tools to provide real-time visibility into team velocity and sprint health.",
    tags: ["Developer Tools", "Jira", "Slack"],
    integrations: ["Jira", "Linear", "Asana", "Slack"],
    howItWorks: [
      "Connects to your project management tool",
      "Tracks ticket status changes and sprint progress",
      "Identifies potential blockers and delays",
      "Posts daily standups and sprint summaries",
    ],
    safetyPermissions: [
      "Read access to project boards and tickets",
      "Posts to designated team channel",
      "Cannot modify tickets or assignments",
    ],
    suggestedPrompts: [
      "Show sprint progress",
      "What are the blockers?",
      "Generate standup summary",
    ],
  },
  "product-research-intel": {
    name: "Product Research Intel",
    category: "Analytics",
    description:
      "Aggregates product research and competitive intelligence from multiple sources.",
    longDescription:
      "Product Research Intel helps product teams stay informed by aggregating research, competitive intelligence, and market trends from multiple sources. It synthesizes information into actionable insights for product strategy.",
    tags: ["Analytics", "Slack", "Notion"],
    integrations: ["Slack", "Notion", "Confluence", "Google Docs"],
    howItWorks: [
      "Monitors configured research sources and competitors",
      "Aggregates and categorizes relevant information",
      "Synthesizes findings into weekly intelligence briefs",
      "Stores insights in your knowledge base",
    ],
    safetyPermissions: [
      "Read access to configured sources only",
      "Write access to designated Notion workspace",
      "Posts summaries to product channel",
    ],
    suggestedPrompts: [
      "Show competitor updates",
      "What are the market trends?",
      "Find research on [topic]",
    ],
  },
};

export function AgentDetail() {
  const { agentSlug } = useParams<{ agentSlug: string }>();
  const { openAuthModal } = useOutletContext<OutletContext>();

  const agent = agentSlug ? agentsData[agentSlug] : null;

  if (!agent) {
    return (
      <div className="max-w-[1000px]">
        <div className="text-center py-12 px-6">
          <h1 className="text-xl font-semibold mb-3">Agent not found</h1>
          <Link
            to="/hire"
            className="inline-block px-4 py-2 bg-gray-800 text-white border border-gray-800 text-sm no-underline"
          >
            Back to Agents
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-[1000px]">
      <Link
        to="/hire"
        className="inline-flex items-center gap-1 text-sm text-gray-700 mb-6"
      >
        <ArrowLeft size={16} />
        Back to Agents
      </Link>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-6">
        <div>
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

          <h1 className="text-2xl font-semibold mb-3">{agent.name}</h1>
          <p className="text-gray-600 text-sm leading-relaxed mb-4">
            {agent.longDescription}
          </p>

          <div className="flex gap-3 mb-8">
            <button
              className="px-4 py-2 bg-gray-800 text-white border border-gray-800 text-sm cursor-pointer"
              onClick={openAuthModal}
            >
              Hire Agent
            </button>
            <button className="px-4 py-2 border border-gray-300 bg-gray-100 text-sm cursor-pointer hover:bg-gray-200">
              Test Agent
            </button>
          </div>

          <section className="mb-6">
            <h2 className="text-base font-semibold mb-3 pb-2 border-b border-gray-300">
              How it works
            </h2>
            <ol className="pl-5 text-gray-600 text-sm list-decimal">
              {agent.howItWorks.map((step, index) => (
                <li key={index} className="mb-2">
                  {step}
                </li>
              ))}
            </ol>
          </section>

          <section className="mb-6">
            <h2 className="text-base font-semibold mb-3 pb-2 border-b border-gray-300">
              Integrations
            </h2>
            <div className="flex flex-wrap gap-2">
              {agent.integrations.map((integration) => (
                <span
                  key={integration}
                  className="px-3 py-1.5 border border-gray-300 text-sm"
                >
                  {integration}
                </span>
              ))}
            </div>
          </section>

          <section className="mb-6">
            <h2 className="text-base font-semibold mb-3 pb-2 border-b border-gray-300">
              Safety & Permissions
            </h2>
            <ul className="pl-5 text-gray-600 text-sm list-disc">
              {agent.safetyPermissions.map((permission, index) => (
                <li key={index} className="mb-1.5">
                  {permission}
                </li>
              ))}
            </ul>
          </section>
        </div>

        <aside className="lg:sticky lg:top-6 h-fit">
          <div className="border border-gray-300 p-4">
            <h3 className="font-semibold text-sm mb-1">Testing in Sandbox</h3>
            <p className="text-gray-600 text-sm mb-3">
              Try this agent with simulated data
            </p>

            <div className="flex flex-col gap-1.5 mb-3">
              {agent.suggestedPrompts.map((prompt) => (
                <button
                  key={prompt}
                  className="p-2 border border-gray-300 bg-white text-sm text-left cursor-pointer hover:bg-gray-50"
                >
                  {prompt}
                </button>
              ))}
            </div>

            <div className="bg-gray-100 border border-gray-300 min-h-[120px] mb-3 flex items-center justify-center">
              <span className="text-gray-500 text-sm text-center px-3">
                Select a prompt to see a mock response
              </span>
            </div>

            <input
              type="text"
              className="w-full p-2 border border-gray-300 text-sm focus:outline-2 focus:outline-gray-800 focus:-outline-offset-2"
              placeholder="Or type your own prompt..."
            />
          </div>
        </aside>
      </div>
    </div>
  );
}
