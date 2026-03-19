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
const DEPLOYMENT_SLACK_FULL_ID = "dep-slack-full-1";
const DEPLOYMENT_SLACK_OVERLAP_ID = "dep-slack-overlap-1";
const DEPLOYMENT_CROSS_ACCOUNT_ID = "dep-cross-acct-1";
const CROSS_ACCOUNT_PUBLISHER = "otheraccount";
const REJECT_BOT_TOKEN = "xoxb-server-reject";

const nowIso = new Date().toISOString();
const latestBuildByAgent: Record<string, string> = {
  [AGENT_APP_TOKEN_ONLY]: "build-123",
  [AGENT_SLACK_FULL]: "build-124",
  [AGENT_SLACK_OVERLAP]: "build-123",
  [AGENT_CROSS_ACCOUNT]: "build-cross-1",
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
    pods: [],
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
    pods: [],
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
    pods: [],
    jobs: [],
  },
];

let deployments = makeInitialDeployments();

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
  ],
  count: 4,
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
      const template = prefilledTemplatesByDeployment[deploymentId as keyof typeof prefilledTemplatesByDeployment];
      if (
        accountName === ACCOUNT &&
        ((deploymentId === DEPLOYMENT_SLACK_FULL_ID && agentName === AGENT_SLACK_FULL) ||
          (deploymentId === DEPLOYMENT_SLACK_OVERLAP_ID && agentName === AGENT_SLACK_OVERLAP) ||
          (deploymentId === DEPLOYMENT_CROSS_ACCOUNT_ID && agentName === AGENT_CROSS_ACCOUNT)) &&
        template
      ) {
        return json(template);
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

    if (pathname === "/api/v1/deploy" && request.method === "POST") {
      const body = (await request.json()) as {
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
      const exists = deployments.some((d) => d.name === deploymentName);
      if (exists) {
        deployments = deployments.map((d) =>
          d.name === deploymentName ? { ...d, build_id: newBuildId } : d,
        );
      } else {
        deployments = [...deployments, {
          id: `dep-${deploymentName}-${Date.now()}`,
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
          pods: [],
          jobs: [],
        }];
      }
      return json({
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
