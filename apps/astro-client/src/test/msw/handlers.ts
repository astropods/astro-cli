import { http, HttpResponse } from 'msw';
import type {
  BlueprintsListResponse,
  Blueprint,
  DeploymentTemplate,
  DeploymentsListResponse,
  DeployResponse,
  UndeployResponse,
  ObservabilitySummaryResponse,
  ObservabilityTracesResponse,
} from '@/lib/api';

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
  variables: {
    OPENAI_API_KEY: { default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI API key for the model provider' },
    SENTRY_DSN: { default: '', targets: ['agent'], secret: false, optional: true, description: 'Sentry DSN for error tracking' },
  },
  editable: ['variables.*.value', 'interfaces.adapters'],
};

export const mockCrossAccountPrefilledTemplate: DeploymentTemplate = {
  spec: 'deployment-template/v1',
  source: { account: 'testuser', name: 'data-analyst', build: 'c3d4e5f6a7b8', registry: 'registry.example.com' },
  target: { runtime: 'kubernetes', display_name: 'Cross-Account Analyst' },
  agent: { image: 'registry.example.com/testuser/data-analyst:c3d4e5f6a7b8', endpoints: { http: { port: 8080 } } },
  variables: {
    OPENAI_API_KEY: { default: '', targets: ['agent'], secret: true, optional: false, description: 'OpenAI API key', value: 'sk-cross-value' },
  },
  editable: ['variables.*.value', 'interfaces.adapters'],
};

export const handlers = [
  // GET /api/v1/agents
  http.get('/api/v1/agents', () => {
    return HttpResponse.json<BlueprintsListResponse>({
      blueprints: mockBlueprints,
      count: mockBlueprints.length,
    });
  }),

  // GET /api/v1/agents/:account/:name/deployment-template/:deploymentId
  http.get('/api/v1/agents/:account/:name/deployment-template/:deploymentId', ({ params }) => {
    if (params.deploymentId === 'dep-cross-account' && params.name === 'data-analyst') {
      return HttpResponse.json(mockCrossAccountPrefilledTemplate);
    }
    return HttpResponse.json({ error: 'not_found' }, { status: 404 });
  }),

  // GET /api/v1/agents/:account/:name/deployment-template
  http.get('/api/v1/agents/:account/:name/deployment-template', ({ params }) => {
    const agent = mockBlueprints.find((a) => a.account === params.account && a.name === params.name);
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(mockTemplate);
  }),

  // GET /api/v1/agents/:account/:name
  http.get('/api/v1/agents/:account/:name', ({ params }) => {
    const agent = mockBlueprints.find((a) => a.account === params.account && a.name === params.name);
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(agent);
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
