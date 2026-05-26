import { http, HttpResponse } from 'msw';
import { BLUEPRINT_LIST_MAX_LIMIT } from '@/lib/blueprint-list-params';
import type {
  BlueprintsListResponse,
  Blueprint,
  DeploymentTemplate,
  DeploymentSpec,
  DeploymentVariable,
  TemplateResponse,
  DeploymentsListResponse,
  DeployResponse,
  UndeployResponse,
  ObservabilitySummaryResponse,
  ObservabilityTracesResponse,
  TraceDetailResponse,
  AccountUsageResponse,
  DeploymentEventsResponse,
  AccountPublic,
  AccountOrgsResponse,
} from '@/lib/api';

function paginateBlueprintList(
  agents: Blueprint[],
  request: Request,
): BlueprintsListResponse {
  const url = new URL(request.url);
  const limit = Number(url.searchParams.get('limit') ?? BLUEPRINT_LIST_MAX_LIMIT);
  const offset = Number(url.searchParams.get('offset') ?? 0);
  const q = url.searchParams.get('q')?.toLowerCase();
  let filtered = agents;
  if (q) {
    filtered = agents.filter(
      (b) =>
        b.name.toLowerCase().includes(q) ||
        b.versions?.some((v) => v.agent_card?.description?.toLowerCase().includes(q)),
    );
  }
  const page = filtered.slice(offset, offset + limit);
  return {
    agents: page,
    count: filtered.length,
    limit,
    offset,
    has_more: offset + page.length < filtered.length,
  };
}

// Fixture data — realistic but minimal
export const mockBlueprints: Blueprint[] = [
  {
    name: 'code-reviewer',
    account: 'testuser',
    registry: 'registry.example.com',
    versions: [
      {
        build_id: 'a1b2c3d4e5f6',
        spec: {
          model: 'gpt-4o',
          integrations: {
            github: { provider: 'GitHub', type: 'tool' },
            slack: { provider: 'Slack', type: 'tool' },
          },
        },
        agent_card: {
          description: 'Automated code review agent that analyzes pull requests for quality and security issues.',
          tags: ['Development', 'Support'],
          integrations: [
            { id: 'github', name: 'GitHub', known: true },
            { id: 'slack', name: 'Slack', known: true },
          ],
        },
        published_at: '2025-02-01T00:00:00Z',
      },
      {
        build_id: 'b2c3d4e5f6a7',
        spec: {
          model: 'gpt-4',
        },
        agent_card: {
          description: 'Automated code review agent.',
          tags: ['Development'],
          integrations: [
            { id: 'github', name: 'GitHub', known: true },
          ],
        },
        published_at: '2025-01-01T00:00:00Z',
      },
    ],
  },
  {
    name: 'data-analyst',
    account: 'testuser',
    registry: 'registry.example.com',
    versions: [
      {
        build_id: 'c3d4e5f6a7b8',
        spec: {
          model: 'claude-3',
          integrations: {
            snowflake: { provider: 'Snowflake', type: 'tool' },
            slack: { provider: 'Slack', type: 'tool' },
            gsheets: { provider: 'Google Sheets', type: 'tool' },
          },
        },
        agent_card: {
          description: 'Analyzes datasets and generates visual reports with actionable insights.',
          tags: ['Data'],
          integrations: [
            { id: 'snowflake', name: 'Snowflake', known: true },
            { id: 'slack', name: 'Slack', known: true },
            { id: 'google-sheets', name: 'Google Sheets', known: true },
          ],
        },
        published_at: '2025-03-01T00:00:00Z',
      },
    ],
  },
];

export const mockDeployments: DeploymentsListResponse = {
  deployments: [
    {
      id: 'dep-code-reviewer',
      name: 'code-reviewer',
      display_name: 'Code Reviewer',
      build_id: 'b2c3d4e5f6a7',
      namespace: 'astro-abc123def456',
      status: 'Running',
      replicas: 1,
      ready: 1,
      created_at: '2025-04-01T00:00:00Z',
      components: ['deployment', 'service'],
    },
    {
      id: 'dep-cross-account',
      name: 'data-analyst',
      display_name: 'Cross-Account Analyst',
      build_id: 'c3d4e5f6a7b8',
      namespace: 'astro-cross999',
      status: 'Running',
      replicas: 1,
      ready: 1,
      created_at: '2025-04-02T00:00:00Z',
      components: ['deployment', 'service'],
    },
    {
      id: 'dep-error-agent',
      name: 'code-reviewer',
      display_name: 'Code Reviewer (broken)',
      build_id: 'a1b2c3d4e5f6',
      namespace: 'astro-err000',
      status: 'Error',
      replicas: 1,
      ready: 0,
      created_at: '2025-04-03T00:00:00Z',
      components: ['deployment'],
    },
  ],
  count: 3,
};

export const mockTemplate: DeploymentTemplate = {
  spec: 'deployment-template/v1',
  source: { account: 'testuser', name: 'code-reviewer', build: 'a1b2c3d4e5f6', registry: 'registry.example.com' },
  target: { runtime: 'kubernetes' },
  agent: { image: 'registry.example.com/testuser/code-reviewer:a1b2c3d4e5f6', endpoints: { http: { port: 8080 } } },
  interfaces: { adapters: [], endpoints: { grpc: { port: 9090, protocol: 'grpc' }, http: { port: 8080, protocol: 'http' } }, auth: { web: { type: 'oidc' } } },
  variables: {
    OPENAI_API_KEY: { default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI API key for the model provider' },
    SENTRY_DSN: { default: '', targets: ['agent'], secret: false, optional: true, description: 'Sentry DSN for error tracking' },
    SLACK_BOT_TOKEN: { default: '', targets: ['interface.slack'], secret: true, optional: true, description: 'Slack bot token', label: 'Slack Bot Token', placeholder: 'xoxb-...' },
    SLACK_APP_TOKEN: { default: '', targets: ['interface.slack'], secret: true, optional: true, description: 'Slack app token', label: 'Slack App Token', placeholder: 'xapp-...' },
    SLACK_CONFIG: {
      default: '', targets: ['interface.slack'], secret: false, optional: true,
      description: 'Slack adapter configuration', datatype: 'object',
      fields: {
        actionable_reactions: { label: 'Actionable Reactions', description: 'Emoji names the bot acts on', placeholder: 'ticket, bug', datatype: 'csv', optional: true },
        allowed_channel_ids: { label: 'Allowed Channel IDs', description: 'Restrict to specific channels', placeholder: 'C12345, C67890', datatype: 'csv', optional: true },
        allowed_user_ids: { label: 'Allowed User IDs', description: 'Restrict to specific users', placeholder: 'U12345, U67890', datatype: 'csv', optional: true },
      },
    },
  },
};

export function wrapTemplateResponse(
  tmpl: DeploymentTemplate,
  reqBody?: { interfaces?: { adapters?: string[] }; variables?: Record<string, { value?: string; ref?: string }>; schedules?: Record<string, string> },
): TemplateResponse {
  const { variables, spec: _spec, ...rest } = tmpl;
  const reqAdapters = reqBody?.interfaces?.adapters;
  const slackSelected = reqAdapters?.includes('slack') ?? false;
  const templateVars: Record<string, { value?: string; ref?: string; targets: string[]; secret?: boolean; optional?: boolean }> = {};
  if (variables) {
    for (const [k, v] of Object.entries(variables)) {
      const override = reqBody?.variables?.[k];
      const isSlackToken = k === 'SLACK_BOT_TOKEN' || k === 'SLACK_APP_TOKEN';
      templateVars[k] = {
        value: override?.value ?? v.value,
        ref: override?.ref ?? v.ref,
        targets: v.targets,
        secret: v.secret,
        optional: isSlackToken ? !slackSelected : v.optional,
      };
    }
  }
  const interfaces = rest.interfaces as Record<string, unknown> | undefined;
  const shapedAdapters = (reqAdapters ?? (interfaces?.adapters as string[] | undefined) ?? []);
  const shapedAuth = interfaces?.auth as { web?: { type?: string } } | undefined;
  const shapedRest = reqAdapters && interfaces
    ? { ...rest, interfaces: { ...interfaces, adapters: reqAdapters } }
    : rest;

  const schemaVars: Record<string, DeploymentVariable> = {};
  if (variables) {
    for (const [k, v] of Object.entries(variables)) {
      const isSlackToken = k === 'SLACK_BOT_TOKEN' || k === 'SLACK_APP_TOKEN';
      schemaVars[k] = { ...v, optional: isSlackToken ? !slackSelected : v.optional };
    }
  }

  const schedules: Record<string, string> = {};
  const ingestion = rest.ingestion as Record<string, { trigger?: { type?: string; schedule?: string } }> | undefined;
  if (ingestion) {
    for (const [name, ing] of Object.entries(ingestion)) {
      if (ing.trigger?.type === 'schedule') {
        schedules[name] = reqBody?.schedules?.[name] ?? ing.trigger.schedule ?? '';
      }
    }
  }

  const errors = Object.entries(templateVars)
    .filter(([, v]) => !v.optional && !v.value && !v.ref)
    .map(([key]) => ({ field: `variables.${key}`, message: 'required variable is empty' }));
  return {
    spec: 'deployment-template/v1',
    template: { ...shapedRest, spec: 'deployment/v1', variables: templateVars } as DeploymentSpec,
    variables: schemaVars,
    interfaces: { adapters: shapedAdapters, auth: shapedAuth },
    schedules,
    validation: { valid: errors.length === 0, errors },
  };
}

export const mockDeploymentEvents: DeploymentEventsResponse = {
  events: [
    {
      type: 'Normal',
      reason: 'Scheduled',
      message: 'Successfully assigned astro-abc123def456/code-reviewer-7f8d9c-xk2lp to node-pool-1',
      object_kind: 'Pod',
      object_name: 'code-reviewer-7f8d9c-xk2lp',
      count: 1,
      first_timestamp: '2025-04-01T00:00:00Z',
      last_timestamp: '2025-04-01T00:00:00Z',
    },
    {
      type: 'Normal',
      reason: 'Pulled',
      message: 'Container image "registry.example.com/testuser/code-reviewer:b2c3d4e5f6a7" already present on machine',
      object_kind: 'Pod',
      object_name: 'code-reviewer-7f8d9c-xk2lp',
      count: 1,
      first_timestamp: '2025-04-01T00:00:01Z',
      last_timestamp: '2025-04-01T00:00:01Z',
    },
    {
      type: 'Warning',
      reason: 'Unhealthy',
      message: 'Readiness probe failed: HTTP probe failed with statuscode: 503',
      object_kind: 'Pod',
      object_name: 'code-reviewer-7f8d9c-xk2lp',
      count: 3,
      first_timestamp: '2025-04-01T00:00:10Z',
      last_timestamp: '2025-04-01T00:00:30Z',
    },
  ],
};

export const handlers = [
  // GET /api/v1/agents
  http.get('/api/v1/agents', ({ request }) => {
    return HttpResponse.json<BlueprintsListResponse>(
      paginateBlueprintList(mockBlueprints, request),
    );
  }),

  // POST /api/v1/agents/:account/:name/deployment-template (interactive POST)
  http.post('/api/v1/agents/:account/:name/deployment-template', async ({ params, request }) => {
    const agent = mockBlueprints.find((a) => a.account === params.account && a.name === params.name);
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    const body = (await request.json().catch(() => ({}))) as Record<string, unknown>;
    return HttpResponse.json(wrapTemplateResponse(mockTemplate, body as { interfaces?: { adapters?: string[] }; variables?: Record<string, { value?: string; ref?: string }>; schedules?: Record<string, string> }));
  }),

  // GET /api/v1/accounts/:account (single account — tests override for 404/custom data)
  // id must match mockAuthContext.accounts[0].id ('acct-1') for testuser so isSelf stays true
  // after background refetch; other accounts get a deterministic id from their name.
  http.get('/api/v1/accounts/:account', ({ params }) => {
    const account = params.account as string;
    return HttpResponse.json<AccountPublic>({
      id: account === 'testuser' ? 'acct-1' : `acct-${account}`,
      name: account,
      type: 'personal',
      created_at: '2025-01-01T00:00:00Z',
      updated_at: '2025-01-01T00:00:00Z',
    });
  }),

  // GET /api/v1/accounts/:account/orgs
  http.get('/api/v1/accounts/:account/orgs', () => {
    return HttpResponse.json<AccountOrgsResponse>({ orgs: [] });
  }),

  // GET /api/v1/accounts/:account/usage
  http.get('/api/v1/accounts/:account/usage', () => {
    return HttpResponse.json<AccountUsageResponse>({
      account_id: 'acct-1',
      period_start: '2025-01-01T00:00:00Z',
      period_end: '2025-02-01T00:00:00Z',
      meters: {
        compute: { usage: 0, quota: 100 },
        agent_builds: { usage: 0 },
        agent_deployments: { usage: 0 },
        agents: { usage: 0 },
      },
    });
  }),

  // GET /api/v1/agents/:account (list account blueprints)
  http.get('/api/v1/agents/:account', ({ params, request }) => {
    const accountBlueprints = mockBlueprints.filter((b) => b.account === params.account);
    return HttpResponse.json<BlueprintsListResponse>(
      paginateBlueprintList(accountBlueprints, request),
    );
  }),

  // GET /api/v1/agents/:account/:name
  http.get('/api/v1/agents/:account/:name', ({ params }) => {
    const agent = mockBlueprints.find((a) => a.account === params.account && a.name === params.name);
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(agent);
  }),

  // GET /api/v1/deployments/:id/events
  http.get('/api/v1/deployments/:id/events', () => {
    return HttpResponse.json<DeploymentEventsResponse>(mockDeploymentEvents);
  }),

  // GET /api/v1/deployments/:id
  http.get('/api/v1/deployments/:id', ({ params }) => {
    const dep = mockDeployments.deployments.find((d) => d.id === params.id);
    if (!dep) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json({ deployment: dep });
  }),

  // GET /api/v1/deployments
  http.get('/api/v1/deployments', () => {
    return HttpResponse.json(mockDeployments);
  }),

  // POST /api/v1/deploy
  http.post('/api/v1/deploy', async ({ request }) => {
    const body = (await request.json()) as { source?: { name?: string; build?: string } };
    const name = body.source?.name ?? 'unknown';
    const buildId = body.source?.build ?? 'a1b2c3d4e5f6';
    return HttpResponse.json<DeployResponse>({
      status: 'deployed',
      name,
      build_id: buildId,
      k8s_namespace: 'user-abc123',
      deployed_at: new Date().toISOString(),
      resources: [{ kind: 'Deployment', name, status: 'created' }],
    });
  }),

  // POST /api/v1/agents/:account/:name/archive
  http.post('/api/v1/agents/:account/:name/archive', () => {
    return new HttpResponse(null, { status: 204 });
  }),

  // GET /api/v1/deployments/:id/observability/summary
  http.get('/api/v1/deployments/:id/observability/summary', () => {
    return HttpResponse.json<ObservabilitySummaryResponse>({
      total_traces: 0,
      time_range: { start: '2025-01-01T00:00:00Z', end: '2025-01-08T00:00:00Z' },
      metrics: { avg_latency_ms: 0, p95_latency_ms: 0, total_tokens: 0, error_rate: 0, traces_per_hour: 0 },
    });
  }),

  // GET /api/v1/deployments/:id/observability/traces
  http.get('/api/v1/deployments/:id/observability/traces', () => {
    return HttpResponse.json<ObservabilityTracesResponse>({
      traces: [],
      total: 0,
      limit: 1,
      offset: 0,
    });
  }),

  // GET /api/v1/deployments/:id/observability/traces/:traceId
  http.get('/api/v1/deployments/:id/observability/traces/:traceId', ({ params }) => {
    return HttpResponse.json<TraceDetailResponse>({
      trace: {
        trace_id: String(params.traceId),
        name: 'mock-trace',
        timestamp: new Date().toISOString(),
        latency_ms: 0,
        total_cost: 0,
        input: '',
        output: '',
      },
      observations: [],
      scores: [],
    });
  }),

  // POST /api/v1/deployments/:id/stop
  http.post('/api/v1/deployments/:id/stop', ({ params }) => {
    return HttpResponse.json({ status: 'stopped', deployment_id: params.id });
  }),

  // POST /api/v1/deployments/:id/restart
  http.post('/api/v1/deployments/:id/restart', () => {
    return HttpResponse.json({ status: 'restarting', pods: ['pod-abc-1', 'pod-abc-2'] });
  }),

  // GET /api/v1/accounts/:account/hearts
  http.get('/api/v1/accounts/:account/hearts', () => {
    return HttpResponse.json({ items: [] });
  }),

  // GET /api/v1/accounts/:account/variables
  http.get('/api/v1/accounts/:account/variables', () => {
    return HttpResponse.json({ variables: [] });
  }),

  // GET /api/v1/accounts/:account/quota-increase
  http.get('/api/v1/accounts/:account/quota-increase', () => {
    return HttpResponse.json({ requests: [] });
  }),

  // POST /api/v1/undeploy
  http.post('/api/v1/undeploy', async ({ request }) => {
    const body = (await request.json()) as { deployment_id: string };
    return HttpResponse.json<UndeployResponse>({
      status: 'undeployed',
      name: 'agent',
      build_id: 'a1b2c3d4e5f6',
      k8s_namespace: 'user-abc123',
      undeployed_at: new Date().toISOString(),
      resources: [{ kind: 'Deployment', name: body.deployment_id, status: 'deleted' }],
    });
  }),

];
