declare const Bun: {
  serve(options: {
    port: number;
    fetch: (request: Request) => Response | Promise<Response>;
  }): unknown;
};

const ACCOUNT = "testuser";
const AGENT_APP_TOKEN_ONLY = "code-reviewer";
const AGENT_SLACK_FULL = "slack-config-full";
const AGENT_SLACK_OVERLAP = "slack-overlap-targets";
const AGENT_CROSS_ACCOUNT = "cross-agent";
const AGENT_INGESTION_SCHEDULE = "ingestion-scheduled";
const AGENT_KNOWLEDGE_BINDINGS = "knowledge-bindings";
const AGENT_XACCT_UPGRADE = "xacct-upgrade-bot";
const AGENT_XACCT_COLLISION = "xacct-collision-bot";
const AGENT_XACCT_PRIVATE = "xacct-private-bot";
const AGENT_ORG_SUPPORT = "org-support-bot";
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const DEPLOYMENT_SLACK_OVERLAP_ID = "dep-slack-overlap-1";
const DEPLOYMENT_CROSS_ACCOUNT_ID = "dep-cross-acct-1";
const DEPLOYMENT_ORG_SUPPORT_ID = "dep-org-support-1";
const CROSS_ACCOUNT_PUBLISHER = "otheraccount";
const DEPLOYMENT_INGESTION_SCHEDULE_ID = "dep-ingestion-schedule-1";
const DEPLOYMENT_KNOWLEDGE_BINDINGS_ID = "dep-knowledge-bindings-1";
const SHARED_POSTGRES_ARN = "arn:astro:knowledge:testuser:shared-postgres";
// Cross-account upgrade fixture (legit upgrade exists in source account).
//
// The deployment was built from CROSS_ACCOUNT_PUBLISHER's blueprint and is
// pinned to its older build. The publisher account has a newer build in
// its blueprint version list; the personal account does NOT have a
// blueprint with this name at all. Pre-fix the client looked up the
// blueprint under the URL/owning account (`testuser`), found nothing,
// and silenced the upgrade badge. Post-fix it follows source_account and
// surfaces the upgrade.
const DEPLOYMENT_XACCT_UPGRADE_ID = "dep-xacct-upgrade-1";
const XACCT_UPGRADE_DEPLOYED_BUILD = "build-xacct-1";
const XACCT_UPGRADE_LATEST_BUILD = "build-xacct-2";
// Cross-account name-collision fixture (no real upgrade; personal account
// has a same-named but lineage-unrelated blueprint with a newer build).
//
// The deployment was built from CROSS_ACCOUNT_PUBLISHER's blueprint and is
// pinned to the publisher's latest build (no upgrade in the source
// lineage). The personal account ALSO publishes a blueprint with the same
// name whose latest build is newer. Pre-fix the client matched by name
// against the personal account's list and advertised that newer build as
// an upgrade - but the server cannot honor it because the deployment's
// build_id is not in that lineage (this is the redeploy-404 trigger
// the user reproduced in production). Post-fix the lookup goes to the
// source account's blueprint and the badge stays silent.
const DEPLOYMENT_XACCT_COLLISION_ID = "dep-xacct-collision-1";
const XACCT_COLLISION_PUBLISHER_BUILD = "build-org-7";
const XACCT_COLLISION_PERSONAL_NEWER = "build-personal-7";
const DEPLOYMENT_XACCT_PRIVATE_ID = "dep-xacct-private-1";
const XACCT_PRIVATE_DEPLOYED_BUILD = "build-private-1";
const XACCT_PRIVATE_LATEST_BUILD = "build-private-2";
const REJECT_BOT_TOKEN = "xoxb-server-reject";
const ORG_ACCOUNT = "test-org";
const ORG_ACCOUNT_ID = "org-acct-1";
const WOS_ORG_ID = "wos-org-1";

const nowIso = new Date().toISOString();
const latestBuildByAgent: Record<string, string> = {
  [AGENT_APP_TOKEN_ONLY]: "build-123",
  [AGENT_SLACK_FULL]: "build-124",
  [AGENT_SLACK_OVERLAP]: "build-123",
  [AGENT_CROSS_ACCOUNT]: "build-cross-1",
  [AGENT_INGESTION_SCHEDULE]: "build-125",
  [AGENT_KNOWLEDGE_BINDINGS]: "build-126",
  // Personal-account "latest" for the collision agent name. Intentionally
  // newer than the deployment's pinned build so the pre-fix
  // (name-only lookup against the viewer's account) would surface a
  // misleading upgrade. The post-fix consults the source account instead.
  [AGENT_XACCT_COLLISION]: XACCT_COLLISION_PERSONAL_NEWER,
};

// Mutable org role - changed via /test/set-role
let currentOrgRole = "admin";
// Mutable unauth flag - changed via /test/set-unauth
let forceUnauth = false;

const makeAuthResponse = () => ({
  user: {
    id: "user-1",
    email: "test@example.com",
    first_name: "Test",
    last_name: "User",
    email_verified: true,
    created_at: nowIso,
    updated_at: nowIso,
  },
  session_id: "session-1",
  organization_id: WOS_ORG_ID,
  role: currentOrgRole,
  permissions: currentOrgRole === "member" ? [] : ["org:manage"],
  expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
  accounts: [
    { id: "acct-1", name: ACCOUNT, type: "personal" },
    { id: ORG_ACCOUNT_ID, name: ORG_ACCOUNT, type: "organization", display_name: "Test Org", organization_id: WOS_ORG_ID, role: currentOrgRole },
  ],
});

const makeOrgMembers = () => [
  {
    account_id: ORG_ACCOUNT_ID,
    user_id: "user-1",
    role: currentOrgRole,
    status: "active",
    username: ACCOUNT,
    display_name: "Test User",
    created_at: nowIso,
  },
  {
    account_id: ORG_ACCOUNT_ID,
    user_id: "user-2",
    role: "member",
    status: "active",
    username: "otheruser",
    display_name: "Other User",
    created_at: nowIso,
  },
];

const baseVariables = {
  OPENAI_API_KEY: {
    default: "",
    targets: ["agent"],
    secret: true,
    optional: false,
    description: "OpenAI API key",
  },
};

const templatesByAgent = {
  [AGENT_APP_TOKEN_ONLY]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_APP_TOKEN_ONLY,
      build: "build-123",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_APP_TOKEN_ONLY}:build-123`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    variables: {
      ...baseVariables,
      SLACK_APP_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack app token",
      },
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [AGENT_SLACK_FULL]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_SLACK_FULL,
      build: "build-123",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_SLACK_FULL}:build-123`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"], auth: { web: { type: "oidc" } } },
    variables: {
      ...baseVariables,
      SLACK_BOT_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack bot token",
      },
      SLACK_APP_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack app token",
      },
      SLACK_CONFIG: {
        default: '{"actionable_reactions":["ticket"],"allowed_channel_ids":["C123"]}',
        value: '{"actionable_reactions":["ticket"],"allowed_channel_ids":["C123"]}',
        targets: ["interface.slack"],
        secret: false,
        optional: true,
        description: "Slack adapter configuration",
        datatype: "object",
        fields: {
          actionable_reactions: { label: "Actionable Reactions", description: "Emoji names the bot acts on", placeholder: "ticket, bug", datatype: "csv", optional: true },
          allowed_channel_ids: { label: "Allowed Channel IDs", description: "Restrict to specific channels", placeholder: "C12345, C67890", datatype: "csv", optional: true },
          allowed_user_ids: { label: "Allowed User IDs", description: "Restrict to specific users", placeholder: "U12345, U67890", datatype: "csv", optional: true },
        },
      },
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [AGENT_SLACK_OVERLAP]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_SLACK_OVERLAP,
      build: "build-123",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_SLACK_OVERLAP}:build-123`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    variables: {
      ...baseVariables,
      SLACK_BOT_TOKEN: {
        default: "",
        targets: ["agent", "interface.slack"],
        secret: true,
        optional: false,
        description: "Slack bot token",
      },
      SLACK_APP_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack app token",
      },
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [AGENT_CROSS_ACCOUNT]: {
    spec: "deployment-template/v1",
    source: {
      account: CROSS_ACCOUNT_PUBLISHER,
      name: AGENT_CROSS_ACCOUNT,
      build: "build-cross-1",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/${CROSS_ACCOUNT_PUBLISHER}/${AGENT_CROSS_ACCOUNT}:build-cross-1`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    variables: {
      ...baseVariables,
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [AGENT_INGESTION_SCHEDULE]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_INGESTION_SCHEDULE,
      build: "build-125",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_INGESTION_SCHEDULE}:build-125`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    ingestion: {
      scheduled: {
        image: `registry.example.com/testuser/${AGENT_INGESTION_SCHEDULE}:build-125`,
        trigger: { type: "schedule", schedule: "" },
        resources: { cpu: "100m", memory: "256Mi" },
      },
    },
    variables: {
      ...baseVariables,
    },
    editable: ["variables.*.value", "interfaces.adapters", "ingestion.*.trigger.schedule"],
  },
  [AGENT_KNOWLEDGE_BINDINGS]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_KNOWLEDGE_BINDINGS,
      build: "build-126",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_KNOWLEDGE_BINDINGS}:build-126`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    knowledge: {
      postgres: { provider: "postgres" },
    },
    variables: {
      ...baseVariables,
    },
    editable: ["variables.*.value", "interfaces.adapters", "bindings.knowledge"],
  },
  // Personal-account template for the collision agent name. Not directly
  // exercised by the badge tests, but kept so configure-page navigation
  // to the personal-side blueprint resolves rather than 404s.
  [AGENT_XACCT_COLLISION]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_XACCT_COLLISION,
      build: XACCT_COLLISION_PERSONAL_NEWER,
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_XACCT_COLLISION}:${XACCT_COLLISION_PERSONAL_NEWER}`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    variables: { ...baseVariables },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
} satisfies Record<string, unknown>;

const prefilledTemplatesByDeployment = {
  [DEPLOYMENT_SLACK_FULL_ID]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_SLACK_FULL,
      build: "build-123",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes", display_name: "Slack Full Bot" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_SLACK_FULL}:build-123`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web", "slack"], auth: { web: { type: "oidc" } } },
    variables: {
      OPENAI_API_KEY: {
        ...baseVariables.OPENAI_API_KEY,
        value: "sk-existing-value",
      },
      SLACK_BOT_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack bot token",
        value: "xoxb-existing-value",
      },
      SLACK_APP_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack app token",
        value: "xapp-existing-value",
      },
      SLACK_CONFIG: {
        default: '{"actionable_reactions":["ticket"],"allowed_channel_ids":["C123"]}',
        targets: ["interface.slack"],
        secret: false,
        optional: true,
        description: "Slack adapter configuration",
        datatype: "object",
        fields: {
          actionable_reactions: { label: "Actionable Reactions", description: "Emoji names the bot acts on", placeholder: "ticket, bug", datatype: "csv", optional: true },
          allowed_channel_ids: { label: "Allowed Channel IDs", description: "Restrict to specific channels", placeholder: "C12345, C67890", datatype: "csv", optional: true },
          allowed_user_ids: { label: "Allowed User IDs", description: "Restrict to specific users", placeholder: "U12345, U67890", datatype: "csv", optional: true },
        },
        value: '{"actionable_reactions":["ticket","bug"],"allowed_channel_ids":["C123","C999"],"allowed_user_ids":["U123","U999"]}',
      },
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [DEPLOYMENT_SLACK_OVERLAP_ID]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_SLACK_OVERLAP,
      build: "build-123",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes", display_name: "Slack Overlap Bot" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_SLACK_OVERLAP}:build-123`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web", "slack"] },
    variables: {
      OPENAI_API_KEY: {
        ...baseVariables.OPENAI_API_KEY,
        value: "sk-overlap-existing-value",
      },
      SLACK_BOT_TOKEN: {
        default: "",
        targets: ["agent", "interface.slack"],
        secret: true,
        optional: false,
        description: "Slack bot token",
        value: "xoxb-overlap-existing-value",
      },
      SLACK_APP_TOKEN: {
        default: "",
        targets: ["interface.slack"],
        secret: true,
        optional: false,
        description: "Slack app token",
        value: "xapp-overlap-existing-value",
      },
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [DEPLOYMENT_CROSS_ACCOUNT_ID]: {
    spec: "deployment-template/v1",
    source: {
      account: CROSS_ACCOUNT_PUBLISHER,
      name: AGENT_CROSS_ACCOUNT,
      build: "build-cross-1",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes", display_name: "Cross Account Agent" },
    agent: {
      image: `registry.example.com/${CROSS_ACCOUNT_PUBLISHER}/${AGENT_CROSS_ACCOUNT}:build-cross-1`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    variables: {
      OPENAI_API_KEY: {
        ...baseVariables.OPENAI_API_KEY,
        value: "sk-cross-existing",
      },
    },
    editable: ["variables.*.value", "interfaces.adapters"],
  },
  [DEPLOYMENT_INGESTION_SCHEDULE_ID]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_INGESTION_SCHEDULE,
      build: "build-125",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes", display_name: "Scheduled Ingestor" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_INGESTION_SCHEDULE}:build-125`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    ingestion: {
      scheduled: {
        image: `registry.example.com/testuser/${AGENT_INGESTION_SCHEDULE}:build-125`,
        trigger: { type: "schedule", schedule: "0 0 * * *" },
        resources: { cpu: "100m", memory: "256Mi" },
      },
    },
    variables: {
      OPENAI_API_KEY: {
        ...baseVariables.OPENAI_API_KEY,
        configured: true,
      },
    },
    editable: ["variables.*.value", "interfaces.adapters", "ingestion.*.trigger.schedule"],
  },
  [DEPLOYMENT_KNOWLEDGE_BINDINGS_ID]: {
    spec: "deployment-template/v1",
    source: {
      account: ACCOUNT,
      name: AGENT_KNOWLEDGE_BINDINGS,
      build: "build-126",
      registry: "registry.example.com",
    },
    target: { runtime: "kubernetes", display_name: "Knowledge Local Bot" },
    agent: {
      image: `registry.example.com/testuser/${AGENT_KNOWLEDGE_BINDINGS}:build-126`,
      endpoints: { http: { port: 8080 } },
    },
    interfaces: { image: "messaging:latest", adapters: ["web"] },
    knowledge: {
      postgres: { provider: "postgres" },
    },
    variables: {
      OPENAI_API_KEY: {
        ...baseVariables.OPENAI_API_KEY,
        configured: true,
      },
    },
    editable: ["variables.*.value", "interfaces.adapters", "bindings.knowledge"],
  },
} satisfies Record<string, unknown>;

// Mirrors the server's populateLatestBuildIDs contract: latest_build_id
// is the newest published build in the deployment's lineage account
// (source_account when set, owning account otherwise), with the
// cross-account private blueprint suppressed because the deploy endpoint
// won't honor it. The dashboard reads this field directly to render the
// upgrade badge - keep these values aligned with what the real server
// would compute or the dashboard tests will silently drift.
const makeInitialDeployments = () => [
  {
    id: DEPLOYMENT_SLACK_FULL_ID,
    name: AGENT_SLACK_FULL,
    display_name: "Slack Full Bot",
    build_id: "build-123",
    latest_build_id: "build-124",
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web", "slack"],
    external_urls: [],
    workloads: [
      {
        name: "slack-config-full-agent",
        kind: "Deployment",
        component: "agent",
        age: "2d",
        containers: [{ name: "agent", state: "running", ready: true as boolean, restart_count: 0 }],
      },
    ],
    jobs: [],
  },
  {
    id: DEPLOYMENT_SLACK_OVERLAP_ID,
    name: AGENT_SLACK_OVERLAP,
    display_name: "Slack Overlap Bot",
    build_id: "build-123",
    latest_build_id: "build-123",
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web", "slack"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_CROSS_ACCOUNT_ID,
    name: AGENT_CROSS_ACCOUNT,
    display_name: "Cross Account Agent",
    build_id: "build-cross-1",
    latest_build_id: "build-cross-1",
    namespace: "astro-cross-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_INGESTION_SCHEDULE_ID,
    name: AGENT_INGESTION_SCHEDULE,
    display_name: "Scheduled Ingestor",
    build_id: "build-125",
    latest_build_id: "build-125",
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    manual_ingestions: ["manual", "full-sync"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_KNOWLEDGE_BINDINGS_ID,
    name: AGENT_KNOWLEDGE_BINDINGS,
    display_name: "Knowledge Local Bot",
    build_id: "build-126",
    latest_build_id: "build-126",
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_XACCT_UPGRADE_ID,
    name: AGENT_XACCT_UPGRADE,
    display_name: "Cross-Account Upgrade Bot",
    build_id: XACCT_UPGRADE_DEPLOYED_BUILD,
    // source_account points at the publisher (otheraccount). The
    // personal account does NOT have a blueprint by this name, so the
    // upgrade signal is observable only when the server's lineage
    // join honors source_account.
    source_account: CROSS_ACCOUNT_PUBLISHER,
    latest_build_id: XACCT_UPGRADE_LATEST_BUILD,
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_XACCT_COLLISION_ID,
    name: AGENT_XACCT_COLLISION,
    display_name: "Cross-Account Collision Bot",
    build_id: XACCT_COLLISION_PUBLISHER_BUILD,
    source_account: CROSS_ACCOUNT_PUBLISHER,
    // Source-account lineage has only the deployed build, so the server
    // reports no upgrade. The personal account's same-named blueprint
    // (with a newer build) must NOT influence this - proven by the
    // dashboard staying silent on this card.
    latest_build_id: XACCT_COLLISION_PUBLISHER_BUILD,
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_XACCT_PRIVATE_ID,
    name: AGENT_XACCT_PRIVATE,
    display_name: "Cross-Account Private Bot",
    build_id: XACCT_PRIVATE_DEPLOYED_BUILD,
    source_account: CROSS_ACCOUNT_PUBLISHER,
    // Cross-account + private source blueprint: the deploy endpoint
    // refuses to honor a redeploy here (canDeploySourceAgent), so the
    // server suppresses latest_build_id rather than advertise a
    // doomed upgrade. Field intentionally omitted.
    namespace: "astro-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
  {
    id: DEPLOYMENT_ORG_SUPPORT_ID,
    name: AGENT_ORG_SUPPORT,
    display_name: "Org Support Bot",
    build_id: "build-org-support-1",
    latest_build_id: "build-org-support-1",
    namespace: "astro-org-namespace",
    status: "healthy",
    replicas: 1,
    ready: 1,
    created_at: nowIso,
    components: ["agent", "web"],
    external_urls: [],
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
    jobs: [],
  },
];

let deployments = makeInitialDeployments();
let storedPayloads: Record<string, Record<string, unknown>> = {};
let createdBlueprints = new Set<string>();

// In-app chat threads keyed by conversation id. Seeded with the demo thread the
// chat specs read; sending a message appends the user turn plus a canned
// assistant reply, and the SSE stream replays that reply chunk by chunk. Keeping
// the persisted thread in sync with the stream is what lets the post-finish
// history refetch (finalizeConversation) land on the same content.
type ChatMessage = { id: string; role: "user" | "assistant"; content: string };
const CHAT_SEED_CONV = "conv-demo-1";
const CHAT_SEED_TITLE = "Trip planning to Lisbon";
const ASSISTANT_REPLY_CHUNKS = ["Sure, ", "here is a quick plan for your trip."];
const ASSISTANT_REPLY = ASSISTANT_REPLY_CHUNKS.join("");
const makeInitialChatThreads = (): Record<string, ChatMessage[]> => ({
  [CHAT_SEED_CONV]: [
    { id: "m1", role: "user", content: "Plan me a weekend in Lisbon." },
    {
      id: "m2",
      role: "assistant",
      content: "Sure! Start in Alfama, then Belem the next morning.",
    },
  ],
});
let chatThreads = makeInitialChatThreads();
let chatMessageSeq = 0;
const nextChatMessageId = (prefix: string) => `${prefix}-${(chatMessageSeq += 1)}`;
const knowledgeStores = [
  {
    id: "ks-shared-postgres",
    arn: SHARED_POSTGRES_ARN,
    name: "shared-postgres",
    provider: "postgres",
    mode: "external",
    status: "ready",
    created_at: nowIso,
    updated_at: nowIso,
  },
];

const orgKnowledgeStores = [
  {
    id: "ks-org-postgres",
    arn: `arn:astro:knowledge:${ORG_ACCOUNT}:org-postgres`,
    name: "org-postgres",
    provider: "postgres",
    mode: "external",
    status: "ready",
    created_at: nowIso,
    updated_at: nowIso,
  },
];

// GitHub state
let githubAccountConnected = false;
let savedCard: { brand: string; last4: string; exp_month: number; exp_year: number } | null = null;
let billingOwesBalance = false;
let githubConnections: Array<{ agent_name: string; repo_full_name: string; created_at: string }> = [];
const githubRepos = [
  { full_name: "testuser/my-repo", default_branch: "main", private: false, permissions: { admin: true } },
  { full_name: "testuser/another-repo", default_branch: "main", private: true, permissions: { admin: true } },
];
let accountVariables: Array<{
  name: string;
  value: string;
  secret: boolean;
  description: string;
  created_at: string;
  updated_at: string;
}> = [];

// Per-agent blueprint version lists, oldest -> newest, for the personal
// account. Default is just `[latestBuildByAgent[name]]`. The collision
// fixture has a personal-account blueprint that is intentionally newer
// than the deployment's source-account pinned build; if the lookup ever
// falls back to the personal account, this is what would (incorrectly)
// appear as the upgrade target.
const versionsByAgent: Record<string, string[]> = {
  [AGENT_SLACK_FULL]: ["build-123", "build-124"],
  [AGENT_XACCT_COLLISION]: ["build-personal-1", XACCT_COLLISION_PERSONAL_NEWER],
};

const buildAgent = (
  account: string,
  agentName: string,
  versionIds: string[],
  visibility = "public",
) => ({
  name: agentName,
  account,
  registry: "registry.example.com",
  visibility,
  // Stagger published_at so the client's "latest" reduce
  // (max by published_at) picks the last entry. Caller passes
  // versions oldest -> newest.
  versions: versionIds.map((build_id, i) => ({
    build_id,
    spec: { model: "gpt-4o" },
    published_at: new Date(Date.parse(nowIso) + i * 1000).toISOString(),
  })),
});

const personalAgentFor = (agentName: string) => {
  const explicit = versionsByAgent[agentName];
  const versionIds = explicit ?? [latestBuildByAgent[agentName] ?? "build-123"];
  return buildAgent(ACCOUNT, agentName, versionIds);
};

const accountAgents = {
  agents: [
    personalAgentFor(AGENT_APP_TOKEN_ONLY),
    personalAgentFor(AGENT_SLACK_FULL),
    personalAgentFor(AGENT_SLACK_OVERLAP),
    personalAgentFor(AGENT_CROSS_ACCOUNT),
    personalAgentFor(AGENT_INGESTION_SCHEDULE),
    personalAgentFor(AGENT_KNOWLEDGE_BINDINGS),
    // Personal-account collision blueprint: same agent_name as the
    // cross-account deployment, totally unrelated lineage, intentionally
    // newer than the deployment's pinned build. Used by the e2e to prove
    // the upgrade signal does NOT come from this blueprint when a
    // cross-account deployment carries source_account.
    personalAgentFor(AGENT_XACCT_COLLISION),
  ],
  count: 7,
};

// Publisher-account (CROSS_ACCOUNT_PUBLISHER) blueprint listing. The
// upgrade-bot has a real newer build in the publisher's lineage; the
// collision-bot has only the deployment's pinned build (no upgrade in
// source). The new client code routes blueprint lookups for cross-
// account deployments here via deployment.source_account.
const publisherAgents = {
  agents: [
    buildAgent(CROSS_ACCOUNT_PUBLISHER, AGENT_XACCT_UPGRADE, [
      XACCT_UPGRADE_DEPLOYED_BUILD,
      XACCT_UPGRADE_LATEST_BUILD,
    ]),
    buildAgent(CROSS_ACCOUNT_PUBLISHER, AGENT_XACCT_COLLISION, [
      XACCT_COLLISION_PUBLISHER_BUILD,
    ]),
    buildAgent(CROSS_ACCOUNT_PUBLISHER, AGENT_XACCT_PRIVATE, [
      XACCT_PRIVATE_DEPLOYED_BUILD,
      XACCT_PRIVATE_LATEST_BUILD,
    ], "private"),
  ],
  count: 3,
};

const orgAgents = {
  agents: [buildAgent(ORG_ACCOUNT, AGENT_ORG_SUPPORT, ["build-org-support-1"], "private")],
  count: 1,
};

const corsHeaders = (origin?: string | null) => ({
  "access-control-allow-origin": origin || "http://127.0.0.1:44317",
  "access-control-allow-credentials": "true",
  "access-control-allow-methods": "GET,POST,PUT,PATCH,DELETE,OPTIONS",
  "access-control-allow-headers": "content-type,authorization",
});

let _currentOrigin: string | null = null;
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json", ...corsHeaders(_currentOrigin) },
  });

function selectedAccounts(url: URL): string[] {
  return url.searchParams.get("scope") === "all"
    ? [ACCOUNT, ORG_ACCOUNT]
    : url.searchParams.getAll("account");
}

function cursorPage<T>(items: T[], url: URL) {
  const limit = Math.max(1, Number(url.searchParams.get("limit")) || 50);
  const offset = Math.max(0, Number(url.searchParams.get("cursor")) || 0);
  const pageItems = items.slice(offset, offset + limit);
  const nextOffset = offset + pageItems.length;
  const hasMore = nextOffset < items.length;
  return {
    items: pageItems,
    page: {
      limit,
      has_more: hasMore,
      ...(hasMore ? { next_cursor: String(nextOffset) } : {}),
    },
  };
}

Bun.serve({
  hostname: "127.0.0.1",
  port: 48787,
  async fetch(request) {
    const url = new URL(request.url);
    const pathname = url.pathname;
    _currentOrigin = request.headers.get("origin");
    if (request.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders(_currentOrigin) });
    }

    if (pathname === "/health") {
      return new Response("ok", { headers: corsHeaders(_currentOrigin) });
    }

    // Reset mutable state between tests so parallel workers don't leak side-effects
    if (pathname === "/test/reset" && request.method === "POST") {
      deployments = makeInitialDeployments();
      storedPayloads = {};
      chatThreads = makeInitialChatThreads();
      chatMessageSeq = 0;
      currentOrgRole = "admin";
      forceUnauth = false;
      createdBlueprints = new Set();
      githubAccountConnected = false;
      savedCard = { brand: "visa", last4: "4242", exp_month: 12, exp_year: 2030 };
      billingOwesBalance = false;
      githubConnections = [];
      accountVariables = [];
      return json({ ok: true });
    }

    // Set the org role for subsequent auth responses
    if (pathname === "/test/set-role" && request.method === "POST") {
      const body = (await request.json()) as { role: string };
      currentOrgRole = body.role;
      return json({ ok: true, role: currentOrgRole });
    }

    // Toggle unauthenticated mode for subsequent auth requests
    // Mirrors the server's 409: removing the card is the other way out of a bill.
    if (pathname === "/test/set-billing-owed" && request.method === "POST") {
      const body = (await request.json()) as { owed: boolean };
      billingOwesBalance = body.owed;
      return json({ ok: true, owed: billingOwesBalance });
    }

    if (pathname === "/test/set-unauth" && request.method === "POST") {
      const body = (await request.json()) as { unauth: boolean };
      forceUnauth = body.unauth;
      return json({ ok: true, unauth: forceUnauth });
    }

    if (pathname === "/auth/me") {
      if (forceUnauth) return json({ error: "unauthorized" }, 401);
      return json(makeAuthResponse());
    }
    if (pathname === "/auth/refresh") {
      if (forceUnauth) return json({ error: "unauthorized" }, 401);
      return json(makeAuthResponse());
    }
    if (pathname === "/auth/switch-org") return json(makeAuthResponse());
    if (pathname === "/auth/login") return new Response("ok");
    if (pathname.startsWith("/auth/logout")) return new Response("ok");

    if (pathname === "/api/v1/me/blueprints" && request.method === "GET") {
      const selected = selectedAccounts(url);
      const query = (url.searchParams.get("q") ?? "").trim().toLowerCase();
      const candidates = [
        ...accountAgents.agents,
        ...orgAgents.agents,
      ].filter((agent) => selected.includes(agent.account));
      const filtered = query
        ? candidates.filter((agent) => agent.name.toLowerCase().includes(query))
        : candidates;
      const ordered = [...filtered].sort((a, b) => a.name.localeCompare(b.name));
      const result = cursorPage(ordered, url);
      return json({
        blueprints: result.items,
        page: result.page,
        scope: { accounts: selected, all: url.searchParams.get("scope") === "all" },
      });
    }

    if (pathname === "/api/v1/me/deployments" && request.method === "GET") {
      const selected = selectedAccounts(url);
      const query = (url.searchParams.get("q") ?? "").trim().toLowerCase();
      const candidates = deployments
        .map((deployment) => ({
          ...deployment,
          messaging_web_configured: true,
          account_id: deployment.id === DEPLOYMENT_ORG_SUPPORT_ID ? ORG_ACCOUNT_ID : "acct-1",
          account_name: deployment.id === DEPLOYMENT_ORG_SUPPORT_ID ? ORG_ACCOUNT : ACCOUNT,
        }))
        .filter((deployment) => selected.includes(deployment.account_name))
        .filter(
          (deployment) =>
            !query ||
            deployment.name.toLowerCase().includes(query) ||
            deployment.display_name.toLowerCase().includes(query),
        );
      const result = cursorPage(candidates, url);
      return json({
        deployments: result.items,
        page: result.page,
        scope: { accounts: selected, all: url.searchParams.get("scope") === "all" },
      });
    }

    if (pathname === "/api/v1/me/knowledge" && request.method === "GET") {
      const selected = selectedAccounts(url);
      const candidates = [
        ...knowledgeStores.map((store) => ({ ...store, account_id: "acct-1", account: ACCOUNT })),
        ...orgKnowledgeStores.map((store) => ({ ...store, account_id: ORG_ACCOUNT_ID, account: ORG_ACCOUNT })),
      ].filter((store) => selected.includes(store.account));
      const result = cursorPage(candidates, url);
      return json({
        stores: result.items,
        page: result.page,
        scope: { accounts: selected, all: url.searchParams.get("scope") === "all" },
      });
    }

    if (pathname === "/api/v1/me/deployment-summaries" && request.method === "GET") {
      const summaries = Object.fromEntries(
        url.searchParams.getAll("deployment").map((id, index) => [id, {
          total_traces: index + 1,
          last_trace_at: nowIso,
          request_series: [0, index + 1],
          token_series: [0, (index + 1) * 100],
          cost_series: [0, (index + 1) / 100],
        }]),
      );
      return json({ summaries });
    }

    const templateMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/deployment-template$/);
    if (templateMatch) {
      const accountName = templateMatch[1];
      const agentName = templateMatch[2];
      if (!accountName || !agentName) return json({ error: "not_found" }, 404);

      // POST: interactive template endpoint - wraps response in TemplateResponse envelope
      if (request.method === "POST") {
        const body = await request.json().catch(() => ({})) as Record<string, unknown>;
        const deploymentId = body.deployment_id as string | undefined;

        // Resolve base template
        let flat: Record<string, unknown> | null = null;
        if (accountName === ACCOUNT && agentName in templatesByAgent) {
          flat = structuredClone(templatesByAgent[agentName as keyof typeof templatesByAgent]) as Record<string, unknown>;
        } else if (accountName === ACCOUNT && createdBlueprints.has(agentName)) {
          flat = {
            spec: "deployment-template/v1",
            source: { account: ACCOUNT, name: agentName, build: "build-new-1", registry: "registry.example.com" },
            target: { runtime: "kubernetes" },
            agent: { image: `registry.example.com/${ACCOUNT}/${agentName}:build-new-1`, endpoints: { http: { port: 8080 } } },
            interfaces: { image: "messaging:latest", adapters: ["web"] },
            variables: { ...baseVariables },
            editable: ["variables.*.value", "interfaces.adapters"],
          };
        }
        if (!flat) return json({ error: "not_found" }, 404);

        // Prefill from stored deployment when deployment_id is provided
        if (deploymentId) {
          const storedPayload = storedPayloads[deploymentId] as Record<string, unknown> | undefined;
          if (storedPayload) {
            flat.target = { ...(flat.target as Record<string, unknown>), deployment_id: deploymentId, display_name: agentName };
            const storedVars = storedPayload.variables as Record<string, Record<string, unknown>> | undefined;
            if (storedVars && flat.variables) {
              const tmplVars = flat.variables as Record<string, Record<string, unknown>>;
              for (const [key, sv] of Object.entries(storedVars)) {
                if (!tmplVars[key]) continue;
                if (sv.secret && sv.value !== undefined && !sv.ref) {
                  tmplVars[key] = { ...tmplVars[key], configured: true, value: undefined, ref: undefined };
                } else if (sv.value !== undefined) {
                  tmplVars[key] = { ...tmplVars[key], value: sv.value };
                } else if (sv.ref) {
                  tmplVars[key] = { ...tmplVars[key], ref: sv.ref, value: undefined };
                }
              }
            }
            const storedIngestion = storedPayload.ingestion as Record<string, Record<string, unknown>> | undefined;
            if (storedIngestion && flat.ingestion) {
              const tmplIngestion = flat.ingestion as Record<string, Record<string, unknown>>;
              for (const [name, si] of Object.entries(storedIngestion)) {
                if (tmplIngestion[name]) {
                  const trigger = si.trigger as Record<string, unknown> | undefined;
                  const tmplTrigger = (tmplIngestion[name] as Record<string, unknown>).trigger as Record<string, unknown>;
                  if (trigger?.schedule) tmplTrigger.schedule = trigger.schedule;
                }
              }
            }
            const storedInterfaces = storedPayload.interfaces as Record<string, unknown> | undefined;
            if (storedInterfaces?.adapters && flat.interfaces) {
              (flat.interfaces as Record<string, unknown>).adapters = storedInterfaces.adapters;
            }
          } else {
            const staticTemplate = prefilledTemplatesByDeployment[deploymentId as keyof typeof prefilledTemplatesByDeployment];
            if (staticTemplate) {
              flat = structuredClone(staticTemplate) as Record<string, unknown>;
            }
          }
        }

        // Merge request-level variable inputs
        const reqVars = body.variables as Record<string, Record<string, unknown>> | undefined;
        if (reqVars && flat.variables) {
          const tmplVars = flat.variables as Record<string, Record<string, unknown>>;
          for (const [key, input] of Object.entries(reqVars)) {
            if (tmplVars[key]) {
              if (input.value !== undefined) { tmplVars[key] = { ...tmplVars[key], value: input.value, ref: undefined }; }
              else if (input.ref) { tmplVars[key] = { ...tmplVars[key], ref: input.ref, value: undefined }; }
            }
          }
        }

        // Merge request-level interfaces (adapters + auth)
        const reqInterfaces = body.interfaces as Record<string, unknown> | undefined;
        const reqAdapters = reqInterfaces?.adapters as string[] | undefined;
        if (reqAdapters && flat.interfaces) {
          (flat.interfaces as Record<string, unknown>).adapters = reqAdapters;
        }
        if (reqInterfaces?.auth && flat.interfaces) {
          (flat.interfaces as Record<string, unknown>).auth = reqInterfaces.auth;
        }

        // Merge request-level schedules
        const reqSchedules = body.schedules as Record<string, string> | undefined;
        if (reqSchedules && flat.ingestion) {
          const tmplIngestion = flat.ingestion as Record<string, Record<string, unknown>>;
          for (const [name, cron] of Object.entries(reqSchedules)) {
            if (tmplIngestion[name]) {
              const trigger = (tmplIngestion[name] as Record<string, unknown>).trigger as Record<string, unknown>;
              if (trigger?.type === "schedule") trigger.schedule = cron;
            }
          }
        }

        // Scrub stored inline secrets from the response (matches server overlay).
        if (flat.variables) {
          const tmplVars = flat.variables as Record<string, Record<string, unknown>>;
          for (const [key, v] of Object.entries(tmplVars)) {
            if (v.secret && v.configured) {
              tmplVars[key] = { ...v, value: undefined, ref: undefined };
            }
          }
        }

        // Build TemplateResponse envelope
        const { editable, variables, ...templateRest } = flat;
        delete templateRest.spec;
        const templateVars: Record<string, unknown> = {};
        if (variables) {
          for (const [k, v] of Object.entries(variables as Record<string, Record<string, unknown>>)) {
            const configuredSecret = !!(v.secret && v.configured);
            templateVars[k] = {
              value: configuredSecret ? undefined : v.value,
              ref: configuredSecret ? undefined : v.ref,
              targets: v.targets,
              secret: v.secret,
              optional: v.optional,
              configured: v.configured,
            };
          }
        }

        // Promote interfaces to response root
        const flatInterfaces = flat.interfaces as Record<string, unknown> | undefined;
        const respInterfaces = {
          adapters: (flatInterfaces?.adapters as string[] | undefined) ?? [],
          auth: flatInterfaces?.auth,
        };

        // Promote schedules to response root
        const respSchedules: Record<string, string> = {};
        const flatIngestion = flat.ingestion as Record<string, Record<string, unknown>> | undefined;
        if (flatIngestion) {
          for (const [name, ing] of Object.entries(flatIngestion)) {
            const trigger = ing.trigger as Record<string, unknown> | undefined;
            if (trigger?.type === "schedule") {
              respSchedules[name] = (trigger.schedule as string) ?? "";
            }
          }
        }

        const errors = variables
          ? Object.entries(variables as Record<string, Record<string, unknown>>)
              .filter(([, v]) => !v.optional && !v.value && !v.ref && !v.configured)
              .map(([key]) => ({ field: `variables.${key}`, message: "required variable is empty" }))
          : [];
        return json({
          spec: "deployment-template/v1",
          template: { ...templateRest, spec: "deployment/v1", variables: templateVars },
          variables: variables ?? {},
          editable: editable ?? [],
          interfaces: respInterfaces,
          schedules: respSchedules,
          validation: { valid: errors.length === 0, errors },
        });
      }

      // GET: legacy flat template (kept for backward compat during transition)
      if (accountName === ACCOUNT && agentName in templatesByAgent) {
        return json(templatesByAgent[agentName as keyof typeof templatesByAgent]);
      }
      if (accountName === ACCOUNT && createdBlueprints.has(agentName)) {
        return json({
          spec: "deployment-template/v1",
          source: { account: ACCOUNT, name: agentName, build: "build-new-1", registry: "registry.example.com" },
          target: { runtime: "kubernetes" },
          agent: { image: `registry.example.com/${ACCOUNT}/${agentName}:build-new-1`, endpoints: { http: { port: 8080 } } },
          interfaces: { image: "messaging:latest", adapters: ["web"] },
          variables: { ...baseVariables },
          editable: ["variables.*.value", "interfaces.adapters"],
        });
      }
      return json({ error: "not_found" }, 404);
    }

    const prefilledTemplateMatch = pathname.match(
      /^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/deployment-template\/([^/]+)$/,
    );
    if (prefilledTemplateMatch) {
      const accountName = prefilledTemplateMatch[1];
      const agentName = prefilledTemplateMatch[2];
      const deploymentId = prefilledTemplateMatch[3];
      if (!accountName || !agentName || !deploymentId) return json({ error: "not_found" }, 404);
      if (accountName !== ACCOUNT) return json({ error: "not_found" }, 404);

      const storedPayload = storedPayloads[deploymentId] as Record<string, unknown> | undefined;
      if (storedPayload && agentName in templatesByAgent) {
        const base = structuredClone(templatesByAgent[agentName as keyof typeof templatesByAgent]);
        const result = base as Record<string, unknown>;
        result.target = { ...(result.target as Record<string, unknown>), deployment_id: deploymentId, display_name: agentName };

        const storedVars = storedPayload.variables as Record<string, Record<string, unknown>> | undefined;
        if (storedVars && result.variables) {
          const tmplVars = result.variables as Record<string, Record<string, unknown>>;
          for (const [key, sv] of Object.entries(storedVars)) {
            if (!tmplVars[key]) continue;
            if (sv.secret && sv.value !== undefined && !sv.ref) {
              tmplVars[key] = { ...tmplVars[key], configured: true, value: undefined, ref: undefined };
            } else if (sv.value !== undefined) {
              tmplVars[key] = { ...tmplVars[key], value: sv.value };
            } else if (sv.ref) {
              tmplVars[key] = { ...tmplVars[key], ref: sv.ref, value: undefined };
            }
          }
        }

        const storedIngestion = storedPayload.ingestion as Record<string, Record<string, unknown>> | undefined;
        if (storedIngestion && result.ingestion) {
          const tmplIngestion = result.ingestion as Record<string, Record<string, unknown>>;
          for (const [name, si] of Object.entries(storedIngestion)) {
            if (tmplIngestion[name]) {
              const trigger = si.trigger as Record<string, unknown> | undefined;
              const tmplTrigger = (tmplIngestion[name] as Record<string, unknown>).trigger as Record<string, unknown>;
              if (trigger?.schedule) {
                tmplTrigger.schedule = trigger.schedule;
              }
            }
          }
        }

        const storedInterfaces = storedPayload.interfaces as Record<string, unknown> | undefined;
        if (storedInterfaces?.adapters && result.interfaces) {
          (result.interfaces as Record<string, unknown>).adapters = storedInterfaces.adapters;
        }

        return json(result);
      }

      const staticTemplate = prefilledTemplatesByDeployment[deploymentId as keyof typeof prefilledTemplatesByDeployment];
      if (staticTemplate) {
        return json(staticTemplate);
      }
      return json({ error: "not_found" }, 404);
    }

    const agentAvatarMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/avatar$/);
    if (agentAvatarMatch && request.method === "POST") {
      const [, accountName, agentName] = agentAvatarMatch;
      return json({ avatar_url: `https://cdn.example.com/${accountName}/${agentName}/avatar.jpg` });
    }

    const agentArchiveMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/archive$/);
    if (agentArchiveMatch && request.method === "POST") {
      const archivedAgent = agentArchiveMatch[2]!;
      githubConnections = githubConnections.filter((c) => c.agent_name !== archivedAgent);
      return json({ ok: true });
    }

    const deploymentHistoryMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/deployment\/history$/);
    if (deploymentHistoryMatch && request.method === "GET") {
      const [, , agentName] = deploymentHistoryMatch;
      const dep = deployments.find((d) => d.name === agentName);
      if (!dep) return json({ deployments: [], count: 0 });
      return json({
        deployments: [
          {
            id: dep.id,
            agent_name: agentName,
            revision: 1,
            build_id: dep.build_id,
            namespace: dep.namespace,
            display_name: dep.display_name,
            is_current: true,
            status: dep.status,
            deployed_at: dep.created_at,
            spec: {},
          },
        ],
        count: 1,
      });
    }

    const agentMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)$/);
    if (agentMatch) {
      const accountName = agentMatch[1];
      const agentName = agentMatch[2];
      if (!accountName || !agentName) return json({ error: "not_found" }, 404);
      if (accountName === ACCOUNT && (agentName in templatesByAgent || createdBlueprints.has(agentName))) {
        return json(personalAgentFor(agentName));
      }
      return json({ error: "not_found" }, 404);
    }

    if (pathname === "/api/v1/agents" && request.method === "GET") {
      return json(accountAgents);
    }

    const accountAgentsMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)$/);
    if (accountAgentsMatch) {
      const [, accountName] = accountAgentsMatch;
      if (request.method === "POST") {
        const body = (await request.json()) as { name: string; visibility?: string };
        createdBlueprints.add(body.name);
        return json({ account: accountName, name: body.name, registry: "registry.example.com", versions: [] }, 201);
      }
      if (accountName === ACCOUNT) {
        return json(accountAgents);
      }
      if (accountName === CROSS_ACCOUNT_PUBLISHER) {
        return json(publisherAgents);
      }
      if (accountName === ORG_ACCOUNT) {
        return json(orgAgents);
      }
      return json({ agents: [], count: 0 });
    }

    if (pathname === "/api/v1/deployments") {
      const accountParam = url.searchParams.get("account");
      if (accountParam === ACCOUNT) {
        // messaging_web_configured marks a deployment chat-eligible (web sidecar).
        const withChat = deployments.map((d) => ({ ...d, messaging_web_configured: true }));
        return json({ deployments: withChat, count: withChat.length });
      }
      return json({ deployments: [], count: 0 });
    }

    // Matched before /deployments/:id so "summary" isn't read as a deployment id.
    if (pathname === "/api/v1/deployments/summary" && request.method === "GET") {
      return json({
        accounts: [
          {
            id: ORG_ACCOUNT_ID,
            name: ACCOUNT,
            type: "user",
            display_name: "Test User",
            deployments: deployments.map((d) => ({
              id: d.id,
              name: d.name,
              display_name: d.display_name,
              status: d.status,
            })),
          },
        ],
      });
    }

    const deploymentDetailMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)$/);
    if (deploymentDetailMatch && request.method === "GET") {
      const dep = deployments.find((d) => d.id === deploymentDetailMatch[1]);
      if (!dep) return json({ error: "not_found" }, 404);
      return json({ deployment: dep });
    }

    // GET /:id/runtime - K8s-derived view. The mock store keeps a single fat
    // workload list on each deployment for historical reasons; project the
    // live-state fields onto a WorkloadRuntime[] keyed by name so the
    // client-side join in AgentDeployments matches the real wire shape.
    const deploymentRuntimeMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/runtime$/);
    if (deploymentRuntimeMatch && request.method === "GET") {
      const dep = deployments.find((d) => d.id === deploymentRuntimeMatch[1]);
      if (!dep) return json({ error: "not_found" }, 404);
      const workloads = (dep.workloads ?? []).map((w) => ({
        name: w.name,
        age: w.age,
        containers: w.containers,
      }));
      const ready = dep.status === "active" ? (dep.replicas ?? 1) : 0;
      return json({
        runtime: {
          ready,
          replicas: dep.replicas ?? 1,
          messaging_reachable: dep.status === "active",
          manual_ingestions: (dep as { manual_ingestions?: string[] }).manual_ingestions,
          workloads,
        },
      });
    }

    // GET /:id/status - server-derived coarse status. Mirrors the handler's
    // DB-precedence ladder: paused/undeploying/failed/pending all short-
    // circuit; active probes the workload (mocked here as ready=replicas).
    const deploymentStatusMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/status$/);
    if (deploymentStatusMatch && request.method === "GET") {
      const dep = deployments.find((d) => d.id === deploymentStatusMatch[1]);
      if (!dep) return json({ error: "not_found" }, 404);
      const s = (dep.status ?? "").toLowerCase();
      if (s === "stopped" || s === "scaled_down") {
        return json({ value: "inactive", reason: "paused", details: "Deployment is paused" });
      }
      if (s === "undeploying") {
        return json({ value: "undeploying", reason: "undeploying", details: "Deployment is being torn down" });
      }
      if (s === "failed" || s === "error") {
        return json({ value: "error", reason: "failed", details: "Deployment failed" });
      }
      if (s === "pending" || s === "provisioning" || s === "deploying") {
        return json({ value: "deploying", reason: "provisioning", details: "Pods are being provisioned" });
      }
      return json({ value: "active", reason: "ready", details: "All replicas ready" });
    }

    // In-app chat: conversation list (titles feed the header + history).
    const chatConversationsMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/chat\/conversations$/,
    );
    if (chatConversationsMatch && request.method === "GET") {
      return json({
        conversations: [
          {
            conversation_id: CHAT_SEED_CONV,
            title: CHAT_SEED_TITLE,
            updated_at: "2026-07-10T12:00:00Z",
          },
        ],
      });
    }

    // A single conversation's messages (renders the thread + its title). Reads
    // the stateful store so a just-sent turn survives the post-stream refetch.
    const chatConversationDetailMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/chat\/conversations\/([^/]+)$/,
    );
    if (chatConversationDetailMatch && request.method === "GET") {
      const convId = chatConversationDetailMatch[2];
      return json({
        conversation_id: convId,
        title: convId === CHAT_SEED_CONV ? CHAT_SEED_TITLE : "New conversation",
        updated_at: "2026-07-10T12:00:00Z",
        messages: chatThreads[convId] ?? [],
        has_more: false,
      });
    }

    // Messaging proxy: create a conversation. The full-page chat generates its
    // own conversation id client-side, so this is only hit when a caller sends
    // without one; return a fresh id and seed an empty thread.
    const messagingCreateConvMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/messaging\/conversations$/,
    );
    if (messagingCreateConvMatch && request.method === "POST") {
      const convId = nextChatMessageId("conv");
      chatThreads[convId] = [];
      return json({ conversation_id: convId, created_at: nowIso });
    }

    // Messaging proxy: send a message. Append the user turn plus the canned
    // assistant reply so the history refetch after the stream finishes matches
    // what was streamed.
    const messagingSendMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/messaging\/conversations\/([^/]+)\/messages$/,
    );
    if (messagingSendMatch && request.method === "POST") {
      const convId = messagingSendMatch[2];
      const body = (await request.json().catch(() => ({}))) as { content?: string };
      const thread = (chatThreads[convId] ??= []);
      thread.push({
        id: nextChatMessageId("u"),
        role: "user",
        content: body.content ?? "",
      });
      thread.push({
        id: nextChatMessageId("a"),
        role: "assistant",
        content: ASSISTANT_REPLY,
      });
      return json({ message_id: nextChatMessageId("m"), timestamp: nowIso });
    }

    // Messaging proxy: cancel an in-flight turn. Nothing to tear down here.
    const messagingCancelMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/messaging\/conversations\/([^/]+)\/cancel$/,
    );
    if (messagingCancelMatch && request.method === "POST") {
      return json({ ok: true });
    }

    // Messaging proxy: SSE response stream. Replays the canned assistant reply
    // as `chunk` events, then a `finish` event, mirroring the sidecar's wire
    // format (see lib/messaging/transport.ts).
    const messagingStreamMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/messaging\/conversations\/([^/]+)\/stream$/,
    );
    if (messagingStreamMatch && request.method === "GET") {
      const enc = new TextEncoder();
      const sse = (event: string, data: unknown) =>
        enc.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
      const stream = new ReadableStream({
        async start(controller) {
          for (const chunk of ASSISTANT_REPLY_CHUNKS) {
            controller.enqueue(sse("chunk", { type: "chunk", content: chunk }));
            await new Promise((r) => setTimeout(r, 40));
          }
          controller.enqueue(sse("finish", { type: "finish" }));
          controller.close();
        },
      });
      return new Response(stream, {
        status: 200,
        headers: {
          "content-type": "text/event-stream",
          "cache-control": "no-cache",
          connection: "keep-alive",
          ...corsHeaders(_currentOrigin),
        },
      });
    }

    // Agent self-reported config, proxied via the messaging sidecar.
    const agentConfigMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/messaging\/agent\/config$/,
    );
    if (agentConfigMatch && request.method === "GET") {
      return json({ systemPrompt: "You are a helpful travel assistant.", tools: [] });
    }

    const deploymentLogsMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/logs$/);
    if (deploymentLogsMatch && request.method === "GET") {
      const lines = [
        { timestamp: "2024-01-01T00:00:00Z", level: null, message: "Starting agent server on :8080" },
        { timestamp: "2024-01-01T00:00:01Z", level: null, message: "Agent ready to accept requests" },
        { timestamp: "2024-01-01T00:00:02Z", level: null, message: "Listening for incoming requests" },
      ];
      // Honor the level filter the way the real backend does. The error-only
      // probe (useLastErrorLog) must not treat these ordinary startup lines as
      // errors, so a level=error request returns nothing for this healthy
      // fixture. The Logs tab sends no level, so it still gets every line.
      const level = url.searchParams.get("level");
      return json(level ? lines.filter((l) => l.level === level) : lines);
    }

    const traceDetailMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/observability\/traces\/(trace-[12])$/,
    );
    if (traceDetailMatch && request.method === "GET") {
      const traceId = traceDetailMatch[2]!;
      const first = traceId === "trace-1";
      return json({
        trace: {
          trace_id: traceId,
          name: first ? "chat completion" : "tool call",
          timestamp: nowIso,
          latency_ms: first ? 523 : 200,
          total_cost: 0,
          input: first ? "What is the weather today?" : "Search for flights to NYC",
          output: first
            ? "I don't have access to real-time weather data."
            : "Found 5 available flights.",
        },
        observations: [],
        scores: [],
      });
    }

    const deploymentObsMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/observability\/(metrics|summary|traces)$/);
    if (deploymentObsMatch && request.method === "GET") {
      const [, , obsType] = deploymentObsMatch;
      if (obsType === "metrics") {
        return json({
          buckets: [
            { timestamp: nowIso, trace_count: 50, avg_latency_ms: 500, p95_latency_ms: 1100, input_tokens: 1000, output_tokens: 800, error_count: 1 },
            { timestamp: nowIso, trace_count: 100, avg_latency_ms: 546, p95_latency_ms: 1200, input_tokens: 2000, output_tokens: 1500, error_count: 2 },
          ],
          time_range: { start: nowIso, end: nowIso },
          interval_minutes: 60,
        });
      }
      if (obsType === "summary") {
        return json({
          total_traces: 150,
          time_range: { start: nowIso, end: nowIso },
          metrics: {
            avg_latency_ms: 523,
            p95_latency_ms: 1200,
            error_rate: 0.02,
            total_tokens: 3500,
            traces_per_hour: 6.25,
          },
        });
      }
      if (obsType === "traces") {
        const traces = [
          {
            trace_id: "trace-1",
            name: "chat completion",
            status: "success",
            latency_ms: 523,
            total_tokens: 150,
            timestamp: nowIso,
          },
          {
            trace_id: "trace-2",
            name: "tool call",
            status: "success",
            latency_ms: 200,
            total_tokens: 80,
            timestamp: nowIso,
          },
        ];
        const search = url.searchParams.get("search")?.trim().toLowerCase();
        const filtered = search
          ? traces.filter((trace) =>
              [trace.trace_id, trace.name].some((value) =>
                value.toLowerCase().includes(search),
              ),
            )
          : traces;
        const limit = Number(url.searchParams.get("limit") ?? 100);
        const offset = Number(url.searchParams.get("offset") ?? 0);
        return json({
          traces: filtered.slice(offset, offset + limit),
          total: filtered.length,
          limit,
          offset,
        });
      }
    }

    const deploymentStopMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/stop$/);
    if (deploymentStopMatch && request.method === "POST") {
      const depId = deploymentStopMatch[1]!;
      deployments = deployments.map((d) =>
        d.id === depId ? { ...d, status: "stopped" } : d,
      );
      return json({ status: "stopped", deployment_id: depId });
    }

    const deploymentCancelMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/cancel$/);
    if (deploymentCancelMatch && request.method === "POST") {
      const depId = deploymentCancelMatch[1]!;
      deployments = deployments.map((d) =>
        d.id === depId ? { ...d, status: "failed" } : d,
      );
      return json({ status: "failed", deployment_id: depId });
    }

    const deploymentWakeupMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/wakeup$/);
    if (deploymentWakeupMatch && request.method === "POST") {
      const depId = deploymentWakeupMatch[1]!;
      deployments = deployments.map((d) =>
        d.id === depId ? { ...d, status: "healthy" } : d,
      );
      return json({ status: "healthy", deployment_id: depId });
    }

    const deploymentRestartMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/restart$/);
    if (deploymentRestartMatch && request.method === "POST") {
      return json({ status: "restarting", pods: [] });
    }

    const triggerMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/ingestion\/([^/]+)\/trigger$/,
    );
    if (triggerMatch && request.method === "POST") {
      const deploymentId = triggerMatch[1]!;
      const ingestionName = triggerMatch[2]!;
      const dep = deployments.find((d) => d.id === deploymentId);
      if (!dep) return json({ error: "not_found" }, 404);
      const jobName = `${dep.name}-ingestion-${ingestionName}-manual`;
      deployments = deployments.map((d) =>
        d.id === deploymentId
          ? {
              ...d,
              workloads: [
                ...(d.workloads ?? []),
                {
                  name: jobName,
                  kind: "Deployment" as const,
                  component: ingestionName,
                  age: "5s",
                  containers: [{ name: ingestionName, state: "running", ready: true as boolean, restart_count: 0 }],
                },
              ],
            }
          : d,
      );
      return json({
        status: "triggered",
        job_name: jobName,
        namespace: dep.namespace,
      });
    }

    if (pathname === "/api/v1/deploy" && request.method === "POST") {
      const body = (await request.json()) as Record<string, unknown> & {
        source?: { name?: string };
        variables?: Record<string, { value?: string }>;
      };
      if (body.variables?.SLACK_BOT_TOKEN?.value === REJECT_BOT_TOKEN) {
        return json(
          {
            error: "validation_failed",
            validation_errors: [
              {
                field: "variables.SLACK_BOT_TOKEN.value",
                message: "required variable has no value",
              },
            ],
          },
          400,
        );
      }
      const deploymentName = body.source?.name ?? AGENT_APP_TOKEN_ONLY;
      const newBuildId = latestBuildByAgent[deploymentName] ?? "build-123";
      const existing = deployments.find((d) => d.name === deploymentName);
      const deploymentId = existing?.id ?? `dep-${deploymentName}-live`;
      if (existing) {
        deployments = deployments.map((d) =>
          d.name === deploymentName ? { ...d, build_id: newBuildId } : d,
        );
      } else {
        deployments = [...deployments, {
          id: deploymentId,
          name: deploymentName,
          display_name: deploymentName,
          build_id: newBuildId,
          namespace: "astro-namespace",
          status: "healthy",
          replicas: 1,
          ready: 1,
          created_at: nowIso,
          components: ["agent", "web"],
          external_urls: [],
          workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
          jobs: [],
        }];
      }
      storedPayloads[deploymentId] = body;
      return json({
        deployment_id: deploymentId,
        status: "deployed",
        name: deploymentName,
        build_id: newBuildId,
        k8s_namespace: "astro-namespace",
        deployed_at: nowIso,
        resources: [{ kind: "Deployment", name: deploymentName, status: "created" }],
      });
    }

    const accountMembersMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/members$/);
    if (accountMembersMatch) {
      const [, accountName] = accountMembersMatch;
      if (request.method === "GET") {
        if (accountName === ORG_ACCOUNT) {
          return json({ members: makeOrgMembers() });
        }
        return json({ members: [] });
      }
      if (request.method === "POST") {
        const body = (await request.json()) as { user_id: string; role: string };
        return json({ member: { account_id: accountName, user_id: body.user_id, role: body.role } }, 201);
      }
    }

    const memberActionMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/members\/([^/]+)$/);
    if (memberActionMatch) {
      if (request.method === "PUT") {
        return json({ message: "role updated" });
      }
      if (request.method === "DELETE") {
        return json({ message: "member removed" });
      }
    }

    const accountInvitationsMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/invitations$/);
    if (accountInvitationsMatch) {
      if (request.method === "GET") {
        return json({ invitations: [] });
      }
      if (request.method === "POST") {
        const body = (await request.json()) as { invitations: { value: string; kind: string; role: string }[] };
        const results = body.invitations.map((inv: { value: string }) => ({ value: inv.value, success: true }));
        return json({ results }, 201);
      }
    }

    const invitationActionMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/invitations\/([^/]+)$/);
    if (invitationActionMatch && request.method === "DELETE") {
      return json({ message: "invitation revoked" });
    }

    const accountAvatarMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/avatar$/);
    if (accountAvatarMatch && request.method === "POST") {
      const [, accountName] = accountAvatarMatch;
      return json({
        avatar_url: `https://cdn.example.com/${accountName}/avatar.jpg`,
        avatar_colors: {
          base: "#111111",
          vibrant: "#222222",
          vibrant_light: "#333333",
          accent: "#444444",
          accent_light: "#555555",
          background: "#666666",
          foreground: "#777777",
          glow: "#888888",
        },
      });
    }

    // Account detail (for display name update, rename, etc.)
    const accountDetailMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)$/);
    if (accountDetailMatch) {
      const [, accountName] = accountDetailMatch;
      if (request.method === "GET") {
        if (accountName === ACCOUNT) {
          return json({ id: "acct-1", name: ACCOUNT, type: "personal", display_name: null });
        }
        if (accountName === ORG_ACCOUNT) {
          return json({ id: ORG_ACCOUNT_ID, name: ORG_ACCOUNT, type: "organization", display_name: "Test Org", organization_id: WOS_ORG_ID });
        }
        return json({ error: "not_found" }, 404);
      }
      if (request.method === "PUT" || request.method === "PATCH") {
        return json({ ok: true });
      }
    }

    const accountVariablesMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/variables$/);
    if (accountVariablesMatch) {
      const accountName = accountVariablesMatch[1]!;
      if (accountName !== ACCOUNT) return json({ error: "not_found" }, 404);

      if (request.method === "GET") {
        return json({
          variables: accountVariables.map((v) => ({
            name: v.name,
            secret: v.secret,
            description: v.description,
            created_at: v.created_at,
            updated_at: v.updated_at,
            ...(v.secret ? {} : { value: v.value }),
          })),
        });
      }

      if (request.method === "POST") {
        const body = (await request.json()) as {
          variables?: Array<{
            name?: string;
            value?: string;
            secret?: boolean;
            description?: string;
          }>;
        };
        const entries = body.variables ?? [];
        if (entries.length === 0) return json({ error: "at least one variable is required" }, 400);
        const results: Array<{ name: string; status: string; error?: string }> = [];
        const ts = new Date().toISOString();
        for (const entry of entries) {
          const name = (entry.name ?? "").trim();
          if (!name) {
            results.push({ name: "", status: "error", error: "name is required" });
            continue;
          }
          const idx = accountVariables.findIndex((v) => v.name === name);
          if (idx !== -1) {
            accountVariables[idx] = {
              ...accountVariables[idx]!,
              value: entry.value ?? "",
              secret: Boolean(entry.secret),
              description: entry.description ?? "",
              updated_at: ts,
            };
          } else {
            accountVariables.unshift({
              name,
              value: entry.value ?? "",
              secret: Boolean(entry.secret),
              description: entry.description ?? "",
              created_at: ts,
              updated_at: ts,
            });
          }
          results.push({ name, status: "created" });
        }
        return json({ results });
      }
    }

    const accountKnowledgeMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/knowledge$/);
    if (accountKnowledgeMatch && request.method === "GET") {
      const accountName = accountKnowledgeMatch[1]!;
      if (accountName !== ACCOUNT) return json([]);
      return json(knowledgeStores);
    }

    const accountVariableItemMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/variables\/([^/]+)$/);
    if (accountVariableItemMatch) {
      const accountName = accountVariableItemMatch[1]!;
      const variableName = decodeURIComponent(accountVariableItemMatch[2]!);
      if (accountName !== ACCOUNT) return json({ error: "not_found" }, 404);

      const idx = accountVariables.findIndex((v) => v.name === variableName);
      if (idx === -1) return json({ error: "not_found" }, 404);

      if (request.method === "DELETE") {
        accountVariables.splice(idx, 1);
        return json({ message: "deleted" });
      }

      if (request.method === "PUT") {
        const body = (await request.json()) as {
          value?: string;
          secret?: boolean;
          description?: string;
        };
        const current = accountVariables[idx]!;
        accountVariables[idx] = {
          ...current,
          value: body.value ?? current.value,
          secret: body.secret ?? current.secret,
          description: body.description ?? current.description,
          updated_at: new Date().toISOString(),
        };
        return json({ name: variableName, message: "updated" });
      }
    }

    const accountObsMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/observability\/summary$/);
    if (accountObsMatch && request.method === "GET") {
      return json({
        period: { start: nowIso, end: nowIso, days: 0 },
        totals: { cost_usd: 0, requests: 0, input_tokens: 0, output_tokens: 0, active_agents: 0 },
        daily_avg: { cost_usd: 0, requests: 0, tokens: 0 },
        cost_over_time: [],
        cost_by_model: [],
      });
    }

    const paymentMethodMatch = pathname.match(
      /^\/api\/v1\/accounts\/([^/]+)\/billing\/payment-method$/,
    );
    if (paymentMethodMatch && request.method === "GET") {
      return json({ available: true, ...(savedCard ? { card: savedCard } : {}) });
    }
    if (paymentMethodMatch && request.method === "DELETE") {
      if (billingOwesBalance) {
        return json(
          {
            error:
              "this account has an outstanding balance; settle it before removing your payment method. To change cards, save the new one instead: it replaces the old card without leaving the account unpayable",
          },
          409,
        );
      }
      savedCard = null;
      return json({ status: "ok" });
    }

    const accountUsageMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/usage$/);
    if (accountUsageMatch && request.method === "GET") {
      return json({
        account_id: "acct-1",
        compute_unit_hours: { usage: 0, quota: 100 },
        agent_builds: { usage: 0 },
        active_deployments: { usage: 0 },
        active_agents: { usage: 0 },
      });
    }

    if (pathname === "/api/v1/accounts/search") {
      return json({ results: [], count: 0 });
    }

    // GitHub account-level endpoints
    // Exact /github match must come before sub-path patterns.
    const githubAccountMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/github$/);
    if (githubAccountMatch) {
      if (request.method === "GET") {
        return json({ connected: githubAccountConnected, github_login: githubAccountConnected ? "testgh" : null });
      }
      if (request.method === "DELETE") {
        githubAccountConnected = false;
        githubConnections = [];
        return json({ ok: true });
      }
    }

    const githubConnectMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/github\/connect$/);
    if (githubConnectMatch && request.method === "POST") {
      const body = (await request.json().catch(() => ({}))) as { redirect_to?: string; force?: boolean };
      if (body.force) {
        return json({ redirect_url: "https://github.com/login/oauth/authorize?mock=1" });
      }
      githubAccountConnected = true;
      return json({ connected: true, github_login: "testgh" });
    }

    const githubReposMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/github\/repos$/);
    if (githubReposMatch && request.method === "GET") {
      return json({ repos: githubRepos });
    }

    const githubConnectionsMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/github\/connections$/);
    if (githubConnectionsMatch && request.method === "GET") {
      return json({ connections: githubConnections });
    }

    const githubScanMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/github\/scan$/);
    if (githubScanMatch && request.method === "GET") {
      return json({ found: false });
    }

    // GitHub agent-level link/unlink: POST|DELETE /api/v1/agents/:account/:name/github/link
    const githubAgentLinkMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/github\/link$/);
    if (githubAgentLinkMatch) {
      const agentName = githubAgentLinkMatch[2]!;
      if (request.method === "POST") {
        const body = (await request.json()) as { repo_full_name: string; branch?: string };
        githubConnections = githubConnections.filter((c) => c.agent_name !== agentName);
        githubConnections.push({ agent_name: agentName, repo_full_name: body.repo_full_name, created_at: nowIso });
        return json({ ok: true });
      }
      if (request.method === "DELETE") {
        githubConnections = githubConnections.filter((c) => c.agent_name !== agentName);
        return json({ ok: true });
      }
    }

    // GitHub agent status/disconnect: GET|DELETE /api/v1/agents/:account/:name/github
    const githubStatusMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/github$/);
    if (githubStatusMatch) {
      const agentName = githubStatusMatch[2]!;
      if (request.method === "GET") {
        const conn = githubConnections.find((c) => c.agent_name === agentName);
        if (!conn) return json({ connected: false, repo_full_name: null, branch: null, builds: [] });
        return json({ connected: true, repo_full_name: conn.repo_full_name, branch: "main", builds: [] });
      }
      if (request.method === "DELETE") {
        githubConnections = githubConnections.filter((c) => c.agent_name !== agentName);
        return json({ ok: true });
      }
    }

    return json({ error: "not_found", path: pathname }, 404);
  },
});

console.log("mock-backend listening on 127.0.0.1:48787");
