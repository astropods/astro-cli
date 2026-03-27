import type { Meta, StoryObj } from "@storybook/react-vite";
import { getIntegrationIconUrl } from "@/lib/assets";

const KNOWN_IDS = [
  "airtable",
  "anthropic",
  "asana",
  "aws",
  "azure",
  "confluence",
  "datadog",
  "discord",
  "dropbox",
  "elasticsearch",
  "figma",
  "gcp",
  "github",
  "gitlab",
  "bitbucket",
  "gmail",
  "google-calendar",
  "google-drive",
  "google-sheets",
  "hubspot",
  "intercom",
  "jira",
  "linear",
  "microsoft-teams",
  "mongodb",
  "notion",
  "openai",
  "pagerduty",
  "pinecone",
  "postgres",
  "qdrant",
  "redis",
  "salesforce",
  "sentry",
  "shopify",
  "slack",
  "snowflake",
  "stripe",
  "trello",
  "twilio",
  "weaviate",
  "zendesk",
];

function IntegrationIconCatalog({ theme }: { theme: "light" | "dark" }) {
  const variant = theme === "dark" ? "dark" : "light";
  return (
    <div className="grid grid-cols-6 gap-4">
      {KNOWN_IDS.map((id) => (
        <div key={id} className="flex flex-col items-center gap-2">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg border bg-muted p-1.5">
            <img
              src={getIntegrationIconUrl(id, variant)}
              alt={id}
              className="h-full w-full object-contain"
            />
          </div>
          <span className="text-xs text-muted-foreground">{id}</span>
        </div>
      ))}
    </div>
  );
}

const meta = {
  title: "Features/Integrations/IntegrationIcons",
  component: IntegrationIconCatalog,
} satisfies Meta<typeof IntegrationIconCatalog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Catalog: Story = {
  args: { theme: "light" },
  render: (_args, { globals }) => (
    <IntegrationIconCatalog theme={globals.theme ?? "light"} />
  ),
};
