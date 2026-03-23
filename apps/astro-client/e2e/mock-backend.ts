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

const nowIso = new Date().toISOString();
const latestBuildByAgent: Record<string, string> = {
  [AGENT_APP_TOKEN_ONLY]: "build-123",
  [AGENT_SLACK_FULL]: "build-124",
  [AGENT_SLACK_OVERLAP]: "build-123",
  [AGENT_CROSS_ACCOUNT]: "build-cross-1",
  [AGENT_INGESTION_SCHEDULE]: "build-125",
};

const authResponse = {
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
  permissions: [],
  expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
  accounts: [{ id: "acct-1", name: ACCOUNT, type: "personal" }],
};

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
    workloads: [] as { name: string; kind: string; component: string; age: string; containers: { name: string; state: string; ready: boolean; restart_count: number }[] }[],
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

function buildMockTraceEntries(deploymentId: string) {
  const now = Date.now();
  return [
    {
      trace_id: `${deploymentId}-trace-001`,
      name: "chat.completion",
      status: "success",
      latency_ms: 412,
      total_tokens: 228,
      input: "Summarize last deployment logs and report errors.",
      output: "Deployment healthy. No critical errors in the last 24 hours.",
      timestamp: new Date(now - 2 * 60 * 1000).toISOString(),
    },
    {
      trace_id: `${deploymentId}-trace-002`,
      name: "chat.completion",
      status: "success",
      latency_ms: 673,
      total_tokens: 346,
      input: "List all missing environment variables.",
      output: "No required variables are missing for this deployment.",
      timestamp: new Date(now - 7 * 60 * 1000).toISOString(),
    },
    {
      trace_id: `${deploymentId}-trace-003`,
      name: "tool.invoke",
      status: "error",
      latency_ms: 1290,
      total_tokens: 190,
      input: "Fetch pod logs for ingestion worker.",
      output: "Failed to fetch pod logs: upstream timeout.",
      timestamp: new Date(now - 13 * 60 * 1000).toISOString(),
    },
    {
      trace_id: `${deploymentId}-trace-004`,
      name: "chat.completion",
      status: "success",
      latency_ms: 532,
      total_tokens: 271,
      input: "Generate a rollout summary for stakeholders.",
      output: "Rollout complete for build-125, all replicas ready.",
      timestamp: new Date(now - 21 * 60 * 1000).toISOString(),
    },
    {
      trace_id: `${deploymentId}-trace-005`,
      name: "chat.completion",
      status: "timeout",
      latency_ms: 3021,
      total_tokens: 0,
      input: "Analyze recent latency outliers.",
      output: "",
      timestamp: new Date(now - 31 * 60 * 1000).toISOString(),
    },
    {
      trace_id: `${deploymentId}-trace-006`,
      name: "tool.invoke",
      status: "success",
      latency_ms: 288,
      total_tokens: 118,
      input: "Read deployment template.",
      output: "Deployment template loaded successfully.",
      timestamp: new Date(now - 42 * 60 * 1000).toISOString(),
    },
    {
      trace_id: `${deploymentId}-trace-007`,
      name: "chat.completion",
      status: "success",
      latency_ms: 756,
      total_tokens: 389,
      input: "Provide a concise incident summary.",
      output: "Transient timeout observed. Service recovered automatically.",
      timestamp: new Date(now - 53 * 60 * 1000).toISOString(),
    },
  ];
}

function buildMockMetrics(traces: ReturnType<typeof buildMockTraceEntries>) {
  const end = new Date();
  const bucketCount = 6;
  const bucketMs = 10 * 60 * 1000;
  const startMs = end.getTime() - bucketCount * bucketMs;

  const buckets = Array.from({ length: bucketCount }, (_, idx) => ({
    timestamp: new Date(startMs + idx * bucketMs).toISOString(),
    trace_count: 0,
    avg_latency_ms: 0,
    p95_latency_ms: 0,
    input_tokens: 0,
    output_tokens: 0,
    error_count: 0,
    _latencies: [] as number[],
  }));

  for (const trace of traces) {
    const ts = new Date(trace.timestamp).getTime();
    if (Number.isNaN(ts) || ts < startMs || ts > end.getTime()) continue;
    const idx = Math.min(bucketCount - 1, Math.floor((ts - startMs) / bucketMs));
    const b = buckets[idx];
    b.trace_count += 1;
    b._latencies.push(trace.latency_ms);
    if (trace.status === "error" || trace.status === "failed") b.error_count += 1;
    const total = trace.total_tokens ?? 0;
    b.input_tokens += Math.round(total * 0.45);
    b.output_tokens += Math.round(total * 0.55);
  }

  return buckets.map((b) => {
    const lats = b._latencies.sort((a, c) => a - c);
    const avg = lats.length ? Math.round(lats.reduce((s, n) => s + n, 0) / lats.length) : 0;
    const p95 = lats.length ? lats[Math.max(0, Math.floor(lats.length * 0.95) - 1)] : 0;
    return {
      timestamp: b.timestamp,
      trace_count: b.trace_count,
      avg_latency_ms: avg,
      p95_latency_ms: p95,
      input_tokens: b.input_tokens,
      output_tokens: b.output_tokens,
      error_count: b.error_count,
    };
  });
}

function buildMockSummary(
  traces: ReturnType<typeof buildMockTraceEntries>,
  startIso: string,
  endIso: string,
) {
  const totalTraces = traces.length;
  const totalLatency = traces.reduce((sum, t) => sum + t.latency_ms, 0);
  const latencies = traces.map((t) => t.latency_ms).sort((a, b) => a - b);
  const totalTokens = traces.reduce((sum, t) => sum + (t.total_tokens ?? 0), 0);
  const errors = traces.filter((t) => t.status === "error" || t.status === "failed").length;
  const durationHours = Math.max(1, (new Date(endIso).getTime() - new Date(startIso).getTime()) / (1000 * 60 * 60));

  return {
    total_traces: totalTraces,
    time_range: { start: startIso, end: endIso },
    metrics: {
      avg_latency_ms: totalTraces ? Math.round(totalLatency / totalTraces) : 0,
      p95_latency_ms: latencies.length ? latencies[Math.max(0, Math.floor(latencies.length * 0.95) - 1)] : 0,
      total_tokens: totalTokens,
      error_rate: totalTraces ? errors / totalTraces : 0,
      traces_per_hour: totalTraces / durationHours,
    },
  };
}

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
      return json({ ok: true });
    }

    if (pathname === "/auth/me") return json(authResponse);
    if (pathname === "/auth/refresh") return json(authResponse);
    if (pathname === "/auth/login") return new Response("ok");
    if (pathname.startsWith("/auth/logout")) return new Response("ok");

    const templateMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)\/deployment-template$/);
    if (templateMatch) {
      const [, accountName, agentName] = templateMatch;
      if (accountName === ACCOUNT && agentName in templatesByAgent) {
        return json(templatesByAgent[agentName as keyof typeof templatesByAgent]);
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

    const agentMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)\/([^/]+)$/);
    if (agentMatch) {
      const [, accountName, agentName] = agentMatch;
      if (accountName === ACCOUNT && agentName in templatesByAgent) {
        return json(agentFor(agentName));
      }
      return json({ error: "not_found" }, 404);
    }

    const accountAgentsMatch = pathname.match(/^\/api\/v1\/agents\/([^/]+)$/);
    if (accountAgentsMatch) {
      const [, accountName] = accountAgentsMatch;
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

    const observabilityMatch = pathname.match(
      /^\/api\/v1\/deployments\/([^/]+)\/observability\/(metrics|summary|traces)$/,
    );
    if (observabilityMatch) {
      const deploymentId = observabilityMatch[1]!;
      const endpoint = observabilityMatch[2]!;
      const traces = buildMockTraceEntries(deploymentId);
      const endIso = url.searchParams.get("end_time") ?? new Date().toISOString();
      const startIso =
        url.searchParams.get("start_time") ??
        new Date(new Date(endIso).getTime() - 60 * 60 * 1000).toISOString();

      if (endpoint === "traces") {
        const limit = Math.max(1, Number(url.searchParams.get("limit") ?? "100"));
        const offset = Math.max(0, Number(url.searchParams.get("offset") ?? "0"));
        const paged = traces.slice(offset, offset + limit);
        return json({
          traces: paged,
          total: traces.length,
          limit,
          offset,
        });
      }

      if (endpoint === "metrics") {
        return json({
          buckets: buildMockMetrics(traces),
          time_range: { start: startIso, end: endIso },
          interval_minutes: 10,
        });
      }

      return json(buildMockSummary(traces, startIso, endIso));
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

    return json({ error: "not_found", path: pathname }, 404);
  },
});

console.log("mock-backend listening on :48787");
