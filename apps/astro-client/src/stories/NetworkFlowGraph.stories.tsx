import type { Meta, StoryObj } from "@storybook/react-vite";
import { NetworkFlowGraph } from "@/components/agent-detail/network/NetworkFlowGraph";
import { getAgentAvatarUrl } from "@/lib/assets";
import type { NetworkFlow } from "@/lib/api";

const SAMPLE_AVATAR = getAgentAvatarUrl("atlas", "code-reviewer");

// Stands in for the server's eTLD+1; adequate for these synthetic hosts.
function etld1(host: string): string | undefined {
  if (/^[\d.]+$/.test(host)) return undefined;
  const parts = host.split(".");
  if (parts.length < 2) return undefined;
  const tail = parts.slice(-2).join(".");
  return parts.length > 2 && /^(co|com|org|net)\.[a-z]{2}$/.test(tail)
    ? parts.slice(-3).join(".")
    : tail;
}

function flow(
  peer: string,
  requestCount: number,
  kind: NetworkFlow["peer_kind"] = "address",
): NetworkFlow {
  return {
    peer,
    peer_kind: kind,
    registrable_domain: kind === "address" ? etld1(peer) : undefined,
    request_count: requestCount,
    error_count: 0,
    error_rate: 0,
    latency_p50_ms: 40,
    latency_p95_ms: 180,
    bytes_total: requestCount * 2_400,
  };
}

const route = (peer: string, requestCount: number) => flow(peer, requestCount, "route");

const meta = {
  title: "Features/Agent Detail/Network/NetworkFlowGraph",
  component: NetworkFlowGraph,
  decorators: [
    (Story) => (
      <div className="mx-auto w-[920px] bg-surface p-6">
        <Story />
      </div>
    ),
  ],
  argTypes: {
    height: { control: { type: "range", min: 200, max: 600, step: 20 } },
    maxBubblesPerSide: { control: { type: "range", min: 3, max: 40, step: 1 } },
  },
} satisfies Meta<typeof NetworkFlowGraph>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    height: 360,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [
      route("/api/chat", 4_210),
      route("/api/tools/invoke", 1_842),
      route("/healthz", 604),
      route("/metrics", 312),
    ],
    outbound: [
      flow("api.openai.com", 1_842),
      flow("api.anthropic.com", 4_210),
      flow("api.github.com", 312),
      flow("api.slack.com", 88),
      flow("api.stripe.com", 27),
    ],
  },
};

export const Sparse: Story = {
  args: {
    height: 320,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [route("/api/chat", 142)],
    outbound: [flow("api.anthropic.com", 142)],
  },
};

export const Dense: Story = {
  args: {
    height: 420,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [
      route("/api/chat", 9_120),
      route("/api/tools/invoke", 2_842),
      route("/api/sessions", 1_204),
      route("/api/files/upload", 612),
      route("/api/feedback", 410),
      route("/healthz", 188),
      route("/metrics", 84),
      route("/api/admin/reload", 19),
    ],
    outbound: [
      flow("api.openai.com", 2_842),
      flow("api.anthropic.com", 9_120),
      flow("api.github.com", 612),
      flow("api.slack.com", 188),
      flow("hooks.slack.com", 410),
      flow("api.stripe.com", 47),
      flow("api.linear.app", 220),
      flow("api.notion.com", 84),
      flow("api.twilio.com", 19),
      flow("events.pagerduty.com", 6),
      flow("api.unknown-vendor.io", 154),
    ],
  },
};

export const UnevenWeights: Story = {
  args: {
    height: 360,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [
      route("/api/chat", 50_000),
      route("/healthz", 12),
      route("/metrics", 3),
    ],
    outbound: [
      flow("api.anthropic.com", 50_000),
      flow("api.openai.com", 12),
      flow("api.github.com", 3),
    ],
  },
};

const KNOWN_HOSTS = [
  "api.openai.com",
  "api.anthropic.com",
  "api.github.com",
  "api.slack.com",
  "hooks.slack.com",
  "api.stripe.com",
  "api.linear.app",
  "api.notion.com",
  "api.twilio.com",
  "events.pagerduty.com",
  "api.figma.com",
  "api.hubapi.com",
];

const FAKE_TLDS = ["com", "io", "net", "ai", "dev", "cloud", "app", "co"];
const FAKE_PREFIXES = ["api", "edge", "ingest", "events", "data", "rpc", "gw", "svc"];

// Deterministic PRNG so the story renders the same on every reload.
function mulberry32(seed: number) {
  return () => {
    seed = (seed + 0x6d2b79f5) >>> 0;
    let t = seed;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function generateOutboundFlows(count: number, seed = 42): NetworkFlow[] {
  const rand = mulberry32(seed);
  const result: NetworkFlow[] = [];

  const whaleCount = Math.min(3, KNOWN_HOSTS.length, count);
  for (let i = 0; i < whaleCount; i++) {
    result.push(flow(KNOWN_HOSTS[i], 80_000 + Math.floor(rand() * 220_000)));
  }

  for (let i = whaleCount; i < Math.min(KNOWN_HOSTS.length, count); i++) {
    result.push(flow(KNOWN_HOSTS[i], 500 + Math.floor(rand() * 6500)));
  }

  while (result.length < count) {
    const prefix = FAKE_PREFIXES[Math.floor(rand() * FAKE_PREFIXES.length)];
    const word = rand().toString(36).slice(2, 6 + Math.floor(rand() * 4));
    const tld = FAKE_TLDS[Math.floor(rand() * FAKE_TLDS.length)];
    // pow(rand, 6) keeps the bulk in the single-digit/low-hundreds range.
    const hits = Math.max(1, Math.floor(Math.pow(rand(), 6) * 4000));
    result.push(flow(`${prefix}.${word}.${tld}`, hits));
  }

  return result;
}

export const LopsidedDenseNetwork: Story = {
  args: {
    height: 520,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [
      route("/api/chat", 12_400),
      route("/api/tools/invoke", 4_800),
      route("/healthz", 2_100),
    ],
    outbound: generateOutboundFlows(200),
  },
};

export const UnknownDestinations: Story = {
  args: {
    height: 320,
    inbound: [route("/api/chat", 240), route("/internal/sync", 64)],
    outbound: [
      flow("api.acme.example", 184),
      flow("data.somewhere.io", 92),
      flow("internal-svc.local", 412),
    ],
  },
};

export const InboundOnly: Story = {
  args: {
    height: 320,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [
      route("/api/chat", 1_204),
      route("/healthz", 320),
      route("/metrics", 96),
    ],
    outbound: [],
  },
};

export const GroupedSubdomains: Story = {
  parameters: {
    docs: {
      description: {
        story:
          "Hover a bubble to see the hosts behind it. The list is capped at five, with the remainder rolled into a \"+N more hosts\" line so the card stays small.",
      },
    },
  },
  args: {
    height: 380,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [route("/api/chat", 8_400), route("/healthz", 240)],
    outbound: [
      flow("api.slack.com", 1_200),
      flow("hooks.slack.com", 3_400),
      flow("files.slack.com", 820),
      flow("api.anthropic.com", 9_120),
      flow("api.github.com", 640),
      flow("uploads.github.com", 210),
      flow("raw.githubusercontent.com", 180),
      // Seven hosts on one vendor — exercises the "+N more hosts" cutoff.
      ...Array.from({ length: 7 }, (_, i) =>
        flow(`shard-${i}.edge.acme-cdn.io`, 900 - i * 90),
      ),
      flow("10.0.14.22", 320),
      flow("internal-svc", 96),
    ],
  },
};

export const Empty: Story = {
  args: {
    height: 320,
    agentAvatarUrl: SAMPLE_AVATAR,
    inbound: [],
    outbound: [],
  },
};
