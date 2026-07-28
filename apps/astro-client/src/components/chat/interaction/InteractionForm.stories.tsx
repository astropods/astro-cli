import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";

import { InteractionForm } from "./InteractionForm";
import type { Interaction } from "./types";

const meta = {
  title: "Chat/Interaction/InteractionForm",
  component: InteractionForm,
  args: {
    onSubmit: fn(),
    onDecline: fn(),
    onCancel: fn(),
    onRespond: fn(),
  },
  decorators: [
    (Story) => (
      <div className="mx-auto max-w-[36rem] p-6">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof InteractionForm>;

export default meta;
type Story = StoryObj<typeof meta>;

function interaction(partial: Partial<Interaction> & Pick<Interaction, "dataSchema" | "actions">): Interaction {
  return { id: "i1", kind: "form", message: "", ...partial };
}

export const TextInput: Story = {
  args: {
    interaction: interaction({
      message: "What's your GitHub username?",
      dataSchema: {
        type: "object",
        properties: { username: { type: "string" } },
        required: ["username"],
      },
      actions: ["submit", "cancel"],
    }),
  },
};

export const SingleSelect: Story = {
  args: {
    interaction: interaction({
      message: "Which environment should I deploy to?",
      dataSchema: {
        type: "object",
        properties: {
          env: {
            type: "string",
            enum: ["dev", "staging", "prod"],
            enumNames: ["Development", "Staging", "Production"],
          },
        },
        required: ["env"],
      },
      actions: ["submit", "cancel"],
    }),
  },
};

export const PickOneOrWriteYourOwn: Story = {
  args: {
    interaction: interaction({
      message: "Pick a base branch, or tell me another.",
      dataSchema: {
        type: "object",
        properties: { branch: { type: "string", enum: ["main", "develop"] } },
        required: ["branch"],
      },
      actions: ["submit", "respond", "cancel"],
    }),
  },
};

export const Confirmation: Story = {
  args: {
    interaction: interaction({
      message: "Delete 4 stale deployments?",
      dataSchema: {
        type: "object",
        properties: { confirm: { type: "boolean", title: "Yes, delete them" } },
        required: ["confirm"],
      },
      actions: ["submit", "cancel"],
    }),
  },
};

export const MultiSelect: Story = {
  args: {
    interaction: interaction({
      message: "Which regions should this run in?",
      dataSchema: {
        type: "object",
        properties: {
          regions: {
            type: "array",
            uniqueItems: true,
            items: { type: "string", enum: ["us-east", "us-west", "eu", "apac"] },
          },
        },
        required: ["regions"],
      },
      actions: ["submit", "cancel"],
    }),
  },
};

export const StructuredForm: Story = {
  args: {
    interaction: interaction({
      message: "Confirm your contact details.",
      dataSchema: {
        type: "object",
        properties: {
          name: { type: "string", title: "Full name" },
          email: { type: "string", format: "email" },
          age: { type: "number", minimum: 18 },
        },
        required: ["name", "email"],
      },
      actions: ["submit", "cancel"],
    }),
  },
};

export const EditAProposal: Story = {
  args: {
    interaction: interaction({
      message: "Review the reply before I send it.",
      dataSchema: {
        type: "object",
        properties: {
          subject: { type: "string" },
          body: { type: "string", "x-ui": { widget: "textarea" } },
        },
        required: ["subject", "body"],
      },
      value: { subject: "Re: invoice", body: "Thanks, received." },
      actions: ["submit", "respond", "cancel"],
    }),
  },
};

// tool_permission with prefilled args; schema.description (LLM-facing) stays hidden.
export const ToolPermission: Story = {
  args: {
    interaction: interaction({
      intent: "tool_permission",
      dataSchema: {
        type: "object",
        title: "set_invoices",
        description: "Insert rows into a database table.",
        properties: {
          table: { type: "string" },
          rows: { type: "integer", minimum: 1 },
        },
        required: ["table", "rows"],
      },
      value: { table: "invoices", rows: 4 },
      actions: ["submit", "decline", "cancel"],
    }),
  },
};

// bash tool_permission: tool-name heading + the command to run.
export const RunCommand: Story = {
  args: {
    interaction: interaction({
      intent: "tool_permission",
      dataSchema: {
        type: "object",
        title: "bash",
        description: "Run a shell command in the workspace.",
        properties: {
          command: { type: "string", title: "Command", "x-ui": { widget: "code" } },
        },
        required: ["command"],
      },
      value: { command: "rm -rf node_modules && pnpm install" },
      actions: ["submit", "decline", "respond"],
    }),
  },
};

export const RunCommandMultiline: Story = {
  args: {
    interaction: interaction({
      intent: "tool_permission",
      dataSchema: {
        type: "object",
        title: "bash",
        description: "Run a shell command in the workspace.",
        properties: {
          command: { type: "string", title: "Command", "x-ui": { widget: "code" } },
        },
        required: ["command"],
      },
      value: {
        command:
          "psql $DATABASE_URL <<'SQL'\nBEGIN;\nALTER TABLE interactions ADD COLUMN responded_at INTEGER;\nCOMMIT;\nSQL",
      },
      actions: ["submit", "decline", "respond"],
    }),
  },
};

// Optional agent-authored message shown as subtext (distinct from schema.description).
export const RunCommandWithDescription: Story = {
  args: {
    interaction: interaction({
      intent: "tool_permission",
      message: "Reinstall dependencies to fix the failing build.",
      dataSchema: {
        type: "object",
        title: "bash",
        properties: {
          command: { type: "string", title: "Command", "x-ui": { widget: "code" } },
        },
        required: ["command"],
      },
      value: { command: "rm -rf node_modules && pnpm install" },
      actions: ["submit", "decline", "respond"],
    }),
  },
};

export const Pending: Story = {
  args: {
    pending: true,
    interaction: TextInput.args!.interaction as Interaction,
  },
};

export const WithError: Story = {
  args: {
    error: "content does not match schema: rows must be ≥ 1",
    interaction: ToolPermission.args!.interaction as Interaction,
  },
};
