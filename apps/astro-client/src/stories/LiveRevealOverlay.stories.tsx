import type { Meta, StoryObj } from "@storybook/react-vite";
import { LiveRevealOverlay } from "@/components/ui/LiveRevealOverlay";
import type { AgentDeploymentSummary } from "@/lib/api";

const mockDeployment: AgentDeploymentSummary = {
  id: "dep-live-reveal-story-1",
  name: "code-reviewer",
  display_name: "Code Reviewer",
  build_id: "build-live-reveal",
  namespace: "storybook",
  status: "pending",
  created_at: new Date().toISOString(),
  // The overlay preloads this into the trading card SVG, which has no <img>
  // error fallback. Point at a committed placeholder rather than the
  // deployment key, whose file is generated per-developer and not in git.
  avatar_url: "/assets/placeholders/accounts/avatar_07.jpg",
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
