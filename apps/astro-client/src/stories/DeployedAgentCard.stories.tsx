import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeployedAgentCard, type DeployedAgentCardProps } from "@/components/DeployedAgentCard";
import type { AvatarColors } from "@/lib/api";

const chrisbotAvatar = "/assets/avatars/agents/chrisjpatty/chrisbot.jpg";
const claudeCodeAvatar = "/assets/avatars/agents/chrisjpatty/claude-code.jpg";
const grapplingHookAvatar = "/assets/avatars/agents/chrisjpatty/grappling-hook.jpg";
const memoryBoxAvatar = "/assets/avatars/agents/chrisjpatty/memory-box.jpg";
const slackBotAvatar = "/assets/avatars/agents/chrisjpatty/slack-bot.jpg";
const dataPipelineAvatar = "/assets/avatars/agents/acme/data-pipeline.jpg";
const weatherBotAvatar = "/assets/avatars/agents/nova/weather-bot.jpg";
const emailResponderAvatar = "/assets/avatars/agents/spark/email-responder.jpg";

const meta = {
  title: "Features/Agents/DeployedAgentCard",
  component: DeployedAgentCard,
} satisfies Meta<DeployedAgentCardProps>;

export default meta;
type Story = StoryObj<DeployedAgentCardProps>;

// Deterministic pseudo-random series so the grid story renders the same every time.
function seededSeries(seed: number, length = 7, base = 20, spread = 30): number[] {
  let s = seed;
  const out: number[] = [];
  for (let i = 0; i < length; i++) {
    s = (s * 1664525 + 1013904223) % 4294967296;
    out.push(Math.round(base + (s / 4294967296) * spread));
  }
  return out;
}

// AvatarColors per avatar — extracted server-side by colorextract.ExtractFromJPEG
// on the actual /assets/avatars/agents/<account>/<name>.jpg files. Do not hand-edit;
// re-derive if the avatars change.
const chrisbotColors: AvatarColors = {
  version: 2, base: "#9bccc7", vibrant: "#2d867c", vibrant_light: "#85e0d6",
  accent: "#82e5da", accent_light: "#99e6dd",
  background: "#0b2220", foreground: "#f4f6f6", glow: "#abede6",
};
const claudeCodeColors: AvatarColors = {
  version: 2, base: "#a93a7b", vibrant: "#862d61", vibrant_light: "#e085bb",
  accent: "#e10285", accent_light: "#d9a6c4",
  background: "#220b19", foreground: "#f6f4f5", glow: "#fa9ed4",
};
const grapplingHookColors: AvatarColors = {
  version: 2, base: "#429cbf", vibrant: "#2d6d86", vibrant_light: "#85c6e0",
  accent: "#03b7fe", accent_light: "#a6cad9",
  background: "#0b1c22", foreground: "#f4f5f6", glow: "#9ee0fa",
};
const memoryBoxColors: AvatarColors = {
  version: 2, base: "#b04f59", vibrant: "#862d37", vibrant_light: "#e0858f",
  accent: "#e01e34", accent_light: "#d7a7aa",
  background: "#220b0e", foreground: "#f6f4f4", glow: "#f3a5ae",
};
const slackBotColors: AvatarColors = {
  version: 2, base: "#b45970", vibrant: "#862d43", vibrant_light: "#e0859b",
  accent: "#e12c59", accent_light: "#d9a6b2",
  background: "#220b11", foreground: "#f6f4f4", glow: "#f2a6b9",
};
const dataPipelineColors: AvatarColors = {
  version: 2, base: "#83a9c9", vibrant: "#2d5e86", vibrant_light: "#85b7e0",
  accent: "#61adeb", accent_light: "#9fe0de",
  background: "#0b1822", foreground: "#f4f5f6", glow: "#a4d0f4",
};
const weatherBotColors: AvatarColors = {
  version: 2, base: "#372120", vibrant: "#86322d", vibrant_light: "#dc8e89",
  accent: "#431714", accent_light: "#d9a9a6",
  background: "#220d0b", foreground: "#f6f4f4", glow: "#e8b4b0",
};
const emailResponderColors: AvatarColors = {
  version: 2, base: "#232b40", vibrant: "#2d4686", vibrant_light: "#859fe0",
  accent: "#14254f", accent_light: "#a6b4d9",
  background: "#0b1222", foreground: "#f4f4f6", glow: "#aebfea",
};

const GRID_CARDS: DeployedAgentCardProps[] = [
  { account: "chrisjpatty", name: "chrisbot", displayName: "Chrisbot", deploymentId: "dep-01HX5K2P9R", avatarUrl: chrisbotAvatar, avatarColors: chrisbotColors, requestSeries: seededSeries(1, 7, 15, 40), tokenSeries: seededSeries(101, 7, 2000, 8000), canLaunch: true },
  { account: "chrisjpatty", name: "claude-code", displayName: "Claude Code", deploymentId: "dep-01HX7M4Q1Z", avatarUrl: claudeCodeAvatar, avatarColors: claudeCodeColors, requestSeries: seededSeries(2, 7, 5, 12), tokenSeries: seededSeries(102, 7, 800, 3000), hasUpdateAvailable: true, latestBuildId: "bld_01HX9CCD3F" },
  { account: "chrisjpatty", name: "grappling-hook", displayName: "Grappling Hook", deploymentId: "dep-01HX9N5R3A", avatarUrl: grapplingHookAvatar, avatarColors: grapplingHookColors, requestSeries: seededSeries(3, 7, 30, 80), tokenSeries: seededSeries(103, 7, 5000, 12000), canLaunch: true },
  { account: "acme", name: "data-pipeline", displayName: "Data Pipeline", deploymentId: "dep-01HXB7T6S4", avatarUrl: dataPipelineAvatar, avatarColors: dataPipelineColors, requestSeries: seededSeries(4, 7, 50, 150), tokenSeries: seededSeries(104, 7, 18000, 40000) },
  { account: "spark", name: "email-responder", displayName: "Email Responder", deploymentId: "dep-01HXD8U7V5", avatarUrl: emailResponderAvatar, avatarColors: emailResponderColors, requestSeries: [0, 0, 0, 0, 0, 0, 0], tokenSeries: [0, 0, 0, 0, 0, 0, 0], hasError: true },
  { account: "chrisjpatty", name: "memory-box", displayName: "Memory Box", deploymentId: "dep-01HXE9V8W6", avatarUrl: memoryBoxAvatar, avatarColors: memoryBoxColors, requestSeries: seededSeries(6, 7, 80, 60), tokenSeries: seededSeries(106, 7, 12000, 20000), canLaunch: true },
  { account: "chrisjpatty", name: "slack-bot", displayName: "Slack Bot", deploymentId: "dep-01HXFAW9X7", avatarUrl: slackBotAvatar, avatarColors: slackBotColors, requestSeries: [0, 0, 0, 0, 0, 0, 14], tokenSeries: [0, 0, 0, 0, 0, 0, 3200], canLaunch: true },
  { account: "nova", name: "weather-bot", displayName: "Weather Bot", deploymentId: "dep-01HXGBX0Y8", avatarUrl: weatherBotAvatar, avatarColors: weatherBotColors, requestSeries: seededSeries(8, 7, 12, 70), tokenSeries: seededSeries(108, 7, 1500, 9000), hasError: true, hasUpdateAvailable: true, latestBuildId: "bld_01HXKQR9TM" },
];

export const Grid: Story = {
  args: {
    account: "acme",
    name: "Incident Command",
  },
  decorators: [
    (Story) => (
      <div className="w-full">
        <Story />
      </div>
    ),
  ],
  render: () => (
    <div className="grid grid-cols-5 gap-4">
      {GRID_CARDS.map((props) => (
        <DeployedAgentCard key={`${props.account}/${props.name}`} {...props} />
      ))}
    </div>
  ),
};
