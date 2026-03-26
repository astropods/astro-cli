import type { Meta, StoryObj } from "@storybook/react-vite";
import { LiveRevealOverlay } from "@/components/deployed-agent/detail/LiveRevealOverlay";
import type { AgentDeployment } from "@/lib/api";

const mockDeployment: AgentDeployment = {
  id: "dep-live-reveal-story-1",
  name: "code-reviewer",
  display_name: "Code Reviewer",
  build_id: "build-live-reveal",
  namespace: "storybook",
  status: "pending",
  replicas: 1,
  ready: 0,
  created_at: new Date().toISOString(),
  components: ["agent"],
};

const meta = {
  title: "Features/DeployedAgent/LiveRevealOverlay",
  component: LiveRevealOverlay,
  parameters: {
    layout: "fullscreen",
  },
  decorators: [
    (Story) => (
      <div className="relative min-h-screen">
        <div className="absolute inset-0 bg-surface p-8 text-muted-foreground">
          Background content placeholder
        </div>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LiveRevealOverlay>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    deployment: mockDeployment,
    // Keep account empty in Storybook so blueprint query is disabled.
    account: "",
    onDismiss: () => {},
    onViewDeployment: () => {},
  },
};
