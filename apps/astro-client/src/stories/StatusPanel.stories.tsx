import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  ActionPanel,
  ErrorPanel,
  InfoPanel,
  NeutralPanel,
  SuccessPanel,
  WarningPanel,
} from "@/components/ui/status-panel";

const meta = {
  title: "Design System/Composites/StatusPanel",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="max-w-xl">
        <Story />
      </div>
    ),
  ],
} satisfies Meta;


export default meta;
type Story = StoryObj<typeof meta>;

/* ── All Tones ── */

export const AllTones: Story = {
  render: () => (
    <div className="space-y-4">
      <ErrorPanel title="Deployment failed">
        Connection to the deployment service timed out. Please try again.
      </ErrorPanel>
      <WarningPanel title="Deployment delayed">
        We are retrying container startup after a temporary infrastructure issue.
      </WarningPanel>
      <InfoPanel title="Heads up">
        Your deployment is still warming up. Some metrics may take a minute to appear.
      </InfoPanel>
      <NeutralPanel title="Information">
        Your agent is currently running in development mode.
      </NeutralPanel>
      <SuccessPanel title="Deployment complete">
        Your latest build is live and healthy across all services.
      </SuccessPanel>
    </div>
  ),
};

export const AllTonesInline: Story = {
  name: "All Tones (Inline)",
  render: () => (
    <div className="space-y-4">
      <ErrorPanel title="Error" variant="inline">
        Connection timed out. Please try again.
      </ErrorPanel>
      <WarningPanel title="Warning" variant="inline">
        Retrying container startup.
      </WarningPanel>
      <InfoPanel title="Info" variant="inline">
        Metrics may take a minute to appear.
      </InfoPanel>
      <NeutralPanel title="Note" variant="inline">
        Running in development mode.
      </NeutralPanel>
      <SuccessPanel title="Success" variant="inline">
        Build is live and healthy.
      </SuccessPanel>
    </div>
  ),
};

/* ── Error ── */

export const Error: Story = {
  render: () => (
    <ErrorPanel title="Deployment failed">
      Connection to the deployment service timed out. Please try again.
    </ErrorPanel>
  ),
};

export const ErrorMultiline: Story = {
  name: "Error — Multiline",
  render: () => (
    <ErrorPanel title="Deployment failed">
      {"SLACK_BOT_TOKEN: invalid token format\nSLACK_APP_TOKEN: token expired\nMissing credentials: DATABASE_URL"}
    </ErrorPanel>
  ),
};

/* ── Warning ── */

export const Warning: Story = {
  render: () => (
    <WarningPanel title="Deployment delayed">
      We are retrying container startup after a temporary infrastructure issue.
    </WarningPanel>
  ),
};

/* ── Info ── */

export const Info: Story = {
  render: () => (
    <InfoPanel title="Heads up">
      Your deployment is still warming up. Some metrics may take a minute to appear.
    </InfoPanel>
  ),
};

/* ── Neutral ── */

export const Neutral: Story = {
  render: () => (
    <NeutralPanel title="Information">
      Your agent is currently running in development mode.
    </NeutralPanel>
  ),
};

/* ── Success ── */

export const Success: Story = {
  render: () => (
    <SuccessPanel title="Deployment complete">
      Your latest build is live and healthy across all services.
    </SuccessPanel>
  ),
};

/* ── Variants ── */

export const WithoutTitle: Story = {
  name: "Without Title",
  render: () => (
    <ErrorPanel>Something went wrong during deployment.</ErrorPanel>
  ),
};

export const Dismissible: Story = {
  render: () => (
    <div className="space-y-4">
      <ErrorPanel title="Deployment failed" dismissible>
        Connection to the deployment service timed out.
      </ErrorPanel>
      <WarningPanel title="Deployment delayed" dismissible>
        Retrying container startup.
      </WarningPanel>
      <SuccessPanel title="Deployment complete" dismissible>
        Build is live and healthy.
      </SuccessPanel>
    </div>
  ),
};

export const InlineDismissible: Story = {
  name: "Inline Dismissible",
  render: () => (
    <div className="space-y-4">
      <ErrorPanel title="Error" variant="inline" dismissible>
        Connection timed out.
      </ErrorPanel>
      <WarningPanel title="Warning" variant="inline" dismissible>
        Retrying startup.
      </WarningPanel>
      <InfoPanel title="Info" variant="inline" dismissible>
        Metrics loading.
      </InfoPanel>
    </div>
  ),
};

/* ── ActionPanel ── */

export const ActionPanelStory: Story = {
  name: "ActionPanel",
  render: () => (
    <div className="space-y-4">
      <ActionPanel
        tone="neutral"
        title="A new version of the CLI is available."
        primaryLabel="Update now"
        onPrimary={() => {}}
      />
      <ActionPanel
        tone="warning"
        title={<>A new build number <span className="font-mono">d4763698</span> is available for this agent.</>}
        primaryLabel="Redeploy →"
        onPrimary={() => {}}
        confirmTitle="Redeploying may be destructive"
        confirmBody="This upstream build may contain breaking changes. Upgrading could affect your agent's behavior or state."
        confirmLabel="Confirm redeploy"
        dismissible
      />
      <ActionPanel
        tone="error"
        title="This deployment is misconfigured and cannot start."
        primaryLabel="Fix now"
        onPrimary={() => {}}
        dismissible
      />
    </div>
  ),
};
