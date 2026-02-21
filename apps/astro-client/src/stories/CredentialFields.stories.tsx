import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { CredentialFields, type CredentialFieldsProps } from "@/components/deploy/CredentialFields";

function CredentialFieldsStateful(props: CredentialFieldsProps) {
  const [values, setValues] = useState<Record<string, string>>(props.values);
  return <CredentialFields {...props} values={values} onChange={setValues} />;
}

const meta = {
  title: "Deploy/CredentialFields",
  component: CredentialFieldsStateful,
  parameters: { layout: "padded" },
} satisfies Meta<typeof CredentialFieldsStateful>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SingleRequired: Story = {
  name: "Single Required Field",
  args: {
    credentials: [
      ["OPENAI_API_KEY", { value: "", description: "OpenAI API key for the model provider", optional: false }],
    ],
    values: {},
    onChange: () => {},
  },
};

export const MultipleFields: Story = {
  name: "Multiple Fields",
  args: {
    credentials: [
      ["OPENAI_API_KEY", { value: "", description: "OpenAI API key for the model provider", optional: false }],
      ["ANTHROPIC_API_KEY", { value: "", description: "Anthropic API key", optional: false }],
      ["DATABASE_URL", { value: "", description: "Postgres connection string", optional: false }],
    ],
    values: {},
    onChange: () => {},
  },
};

export const WithOptionalFields: Story = {
  name: "With Optional Fields",
  args: {
    credentials: [
      ["SLACK_WEBHOOK_URL", { value: "", description: "Webhook for notifications", optional: true }],
      ["SENTRY_DSN", { value: "", description: "Sentry DSN for error tracking", optional: true }],
    ],
    values: {},
    onChange: () => {},
  },
};

export const WithHelpLinks: Story = {
  name: "With Help Links",
  args: {
    credentials: [
      [
        "SLACK_BOT_TOKEN",
        {
          value: "",
          description: "Slack bot token for messaging",
          optional: false,
          label: "Slack Bot Token",
          placeholder: "xoxb-your-token",
          helpUrl: "https://docs.slack.dev/authentication/tokens/",
        },
      ],
      [
        "SLACK_APP_TOKEN",
        {
          value: "",
          description: "Slack app token for socket mode",
          optional: false,
          label: "Slack App Token",
          placeholder: "xapp-your-token",
          helpUrl: "https://docs.slack.dev/authentication/tokens/",
        },
      ],
    ],
    values: {},
    onChange: () => {},
  },
};

export const Prefilled: Story = {
  name: "Prefilled Values",
  args: {
    credentials: [
      ["OPENAI_API_KEY", { value: "", description: "OpenAI API key", optional: false }],
      ["DATABASE_URL", { value: "", description: "Postgres connection string", optional: false }],
    ],
    values: {
      OPENAI_API_KEY: "sk-abc123...",
      DATABASE_URL: "postgres://localhost:5432/mydb",
    },
    onChange: () => {},
  },
};
