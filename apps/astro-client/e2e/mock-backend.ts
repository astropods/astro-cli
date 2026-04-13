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
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const DEPLOYMENT_SLACK_OVERLAP_ID = "dep-slack-overlap-1";
const DEPLOYMENT_CROSS_ACCOUNT_ID = "dep-cross-acct-1";
const CROSS_ACCOUNT_PUBLISHER = "otheraccount";
const DEPLOYMENT_INGESTION_SCHEDULE_ID = "dep-ingestion-schedule-1";
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
};

// Mutable org role — changed via /test/set-role
let currentOrgRole = "admin";

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
    { id: ORG_ACCOUNT_ID, name: ORG_ACCOUNT, type: "organization", display_name: "Test Org", organization_id: WOS_ORG_ID },
  ],
});

// Keep a reference for backwards compat — existing tests use the const
const authResponse = makeAuthResponse();

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
    interfaces: { adapters: ["web"] },
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
    interfaces: { adapters: ["web"] },
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
        description: "Slack adapter configuration as JSON",
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
    interfaces: { adapters: ["web"] },
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
    interfaces: { adapters: ["web"] },
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
    interfaces: { adapters: ["web"] },
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
    interfaces: { adapters: ["web", "slack"] },
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
        description: "Slack adapter configuration as JSON",
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
    interfaces: { adapters: ["web", "slack"] },
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
    interfaces: { adapters: ["web"] },
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
    interfaces: { adapters: ["web"] },
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
        value: "sk-ingest-key",
      },
    },
    editable: ["variables.*.value", "interfaces.adapters", "ingestion.*.trigger.schedule"],
  },
} satisfies Record<string, unknown>;

const makeInitialDeployments = () => [
  {
    id: DEPLOYMENT_SLACK_FULL_ID,
    name: AGENT_SLACK_FULL,
    display_name: "Slack Full Bot",
    build_id: "build-123",
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
];

let deployments = makeInitialDeployments();
let storedPayloads: Record<string, Record<string, unknown>> = {};
let createdBlueprints = new Set<string>();

const agentFor = (agentName: string) => ({
  name: agentName,
  account: ACCOUNT,
  registry: "registry.example.com",
  versions: [
    {
      build_id: latestBuildByAgent[agentName] ?? "build-123",
      spec: { model: "gpt-4o" },
      published_at: nowIso,
    },
  ],
});

const accountAgents = {
  agents: [
    agentFor(AGENT_APP_TOKEN_ONLY),
    agentFor(AGENT_SLACK_FULL),
    agentFor(AGENT_SLACK_OVERLAP),
    agentFor(AGENT_CROSS_ACCOUNT),
    agentFor(AGENT_INGESTION_SCHEDULE),
  ],
  count: 5,
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });

Bun.serve({
  port: 48787,
  async fetch(request) {
    const url = new URL(request.url);
    const pathname = url.pathname;

    if (pathname === "/health") return new Response("ok");

    // Reset mutable state between tests so parallel workers don't leak side-effects
    if (pathname === "/test/reset" && request.method === "POST") {
      deployments = makeInitialDeployments();
      storedPayloads = {};
      currentOrgRole = "admin";
      createdBlueprints = new Set();
      return json({ ok: true });
    }

    // Set the org role for subsequent auth responses
    if (pathname === "/test/set-role" && request.method === "POST") {
      const body = (await request.json()) as { role: string };
      currentOrgRole = body.role;
      return json({ ok: true, role: currentOrgRole });
    }

    if (pathname === "/auth/me") return json(makeAuthResponse());
    if (pathname === "/auth/refresh") return json(makeAuthResponse());
    if (pathname === "/auth/switch-org") return json(makeAuthResponse());
    if (pathname === "/auth/login") return new Response("ok");
    if (pathname.startsWith("/auth/logout")) return new Response("ok");

    const templateMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/deployment-template$/);
    if (templateMatch) {
      const [, accountName, agentName] = templateMatch;
      if (accountName === ACCOUNT && agentName in templatesByAgent) {
        return json(templatesByAgent[agentName as keyof typeof templatesByAgent]);
      }
      // Fallback template for newly created blueprints
      if (accountName === ACCOUNT && createdBlueprints.has(agentName)) {
        return json({
          spec: "deployment-template/v1",
          source: { account: ACCOUNT, name: agentName, build: "build-new-1", registry: "registry.example.com" },
          target: { runtime: "kubernetes" },
          agent: { image: `registry.example.com/${ACCOUNT}/${agentName}:build-new-1`, endpoints: { http: { port: 8080 } } },
          interfaces: { adapters: ["web"] },
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
      const [, accountName, agentName, deploymentId] = prefilledTemplateMatch;
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
            if (tmplVars[key] && sv.value !== undefined) {
              tmplVars[key] = { ...tmplVars[key], value: sv.value };
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
      const [, accountName, agentName] = agentMatch;
      if (accountName === ACCOUNT && (agentName in templatesByAgent || createdBlueprints.has(agentName))) {
        return json(agentFor(agentName));
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
      return json({ agents: [], count: 0 });
    }

    if (pathname === "/api/v1/deployments") {
      const accountParam = url.searchParams.get("account");
      if (accountParam === ACCOUNT) {
        return json({ deployments, count: deployments.length });
      }
      return json({ deployments: [], count: 0 });
    }

    const deploymentDetailMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)$/);
    if (deploymentDetailMatch && request.method === "GET") {
      const dep = deployments.find((d) => d.id === deploymentDetailMatch[1]);
      if (!dep) return json({ error: "not_found" }, 404);
      return json({ deployment: dep });
    }

    const deploymentLogsMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/logs$/);
    if (deploymentLogsMatch && request.method === "GET") {
      const logs = [
        "2024-01-01T00:00:00Z Starting agent server on :8080",
        "2024-01-01T00:00:01Z Agent ready to accept requests",
        "2024-01-01T00:00:02Z Listening for incoming requests",
      ].join("\n");
      return new Response(logs, { status: 200, headers: { "content-type": "text/plain" } });
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
        return json({
          traces: [
            {
              trace_id: "trace-1",
              name: "chat completion",
              status: "success",
              latency_ms: 523,
              total_tokens: 150,
              input: "What is the weather today?",
              output: "I don't have access to real-time weather data.",
              timestamp: nowIso,
            },
            {
              trace_id: "trace-2",
              name: "tool call",
              status: "success",
              latency_ms: 200,
              total_tokens: 80,
              input: "Search for flights to NYC",
              output: "Found 5 available flights.",
              timestamp: nowIso,
            },
          ],
          total: 2,
          limit: 100,
          offset: 0,
        });
      }
    }

    const deploymentStopMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/stop$/);
    if (deploymentStopMatch && request.method === "POST") {
      const depId = deploymentStopMatch[1]!;
      deployments = deployments.map((d) =>
        d.id === depId ? { ...d, status: "stopped", replicas: 0, ready: 0 } : d,
      );
      return json({ status: "stopped", deployment_id: depId });
    }

    const deploymentWakeupMatch = pathname.match(/^\/api\/v1\/deployments\/([^/]+)\/wakeup$/);
    if (deploymentWakeupMatch && request.method === "POST") {
      const depId = deploymentWakeupMatch[1]!;
      deployments = deployments.map((d) =>
        d.id === depId ? { ...d, status: "healthy", replicas: 1, ready: 1 } : d,
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

    const accountObsMatch = pathname.match(/^\/api\/v1\/accounts\/([^/]+)\/observability\/summary$/);
    if (accountObsMatch && request.method === "GET") {
      return json({
        total_traces: 0,
        input_tokens: 0,
        output_tokens: 0,
        time_range: { start: nowIso, end: nowIso },
      });
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

    return json({ error: "not_found", path: pathname }, 404);
  },
});

console.log("mock-backend listening on :48787");
