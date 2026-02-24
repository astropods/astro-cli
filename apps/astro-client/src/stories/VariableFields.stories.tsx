import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";

import { VariableFields, type VariableFieldsProps } from "@/components/deploy/VariableFields";

function VariableFieldsStateful(props: VariableFieldsProps) {
  const [values, setValues] = useState<Record<string, string>>(props.values);
  return <VariableFields {...props} values={values} onChange={setValues} />;
}

const meta = {
  title: "Deploy/VariableFields",
  component: VariableFieldsStateful,
  parameters: { layout: "padded" },
} satisfies Meta<typeof VariableFieldsStateful>;

export default meta;
type Story = StoryObj<typeof meta>;

export const SingleRequired: Story = {
  name: "Single Required Field",
  args: {
    variables: [
      ["OPENAI_API_KEY", { description: "OpenAI API key for the model provider", optional: false }],
    ],
    values: {},
    onChange: () => {},
  },
};

export const MultipleFields: Story = {
  name: "Multiple Fields",
  args: {
    variables: [
      ["OPENAI_API_KEY", { description: "OpenAI API key for the model provider", optional: false }],
      ["ANTHROPIC_API_KEY", { description: "Anthropic API key", optional: false }],
      ["DATABASE_URL", { description: "Postgres connection string", optional: false }],
    ],
    values: {},
    onChange: () => {},
  },
};

export const WithOptionalFields: Story = {
  name: "With Optional Fields",
  args: {
    variables: [
      ["SLACK_WEBHOOK_URL", { description: "Webhook for notifications", optional: true }],
      ["SENTRY_DSN", { description: "Sentry DSN for error tracking", optional: true }],
    ],
    values: {},
    onChange: () => {},
  },
};

export const WithHelpLinks: Story = {
  name: "With Help Links",
  args: {
    variables: [
      [
        "SLACK_BOT_TOKEN",
        {
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
    variables: [
      ["OPENAI_API_KEY", { description: "OpenAI API key", optional: false }],
      ["DATABASE_URL", { description: "Postgres connection string", optional: false }],
    ],
    values: {
      OPENAI_API_KEY: "sk-abc123...",
      DATABASE_URL: "postgres://localhost:5432/mydb",
    },
    onChange: () => {},
  },
};
