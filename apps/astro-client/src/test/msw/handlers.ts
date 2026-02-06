import { http, HttpResponse } from 'msw';
import type {
  AgentsListResponse,
  Agent,
  DeploymentsListResponse,
  DeployResponse,
  UndeployResponse,
} from '@/lib/api';

// Fixture data — realistic but minimal
export const mockAgents: Agent[] = [
  {
    name: 'code-reviewer',
    registry: 'registry.example.com',
    versions: [
      { version: '1.0.0', spec: { model: 'gpt-4' }, published_at: '2025-01-01T00:00:00Z' },
      { version: '1.1.0', spec: { model: 'gpt-4o' }, published_at: '2025-02-01T00:00:00Z' },
    ],
  },
  {
    name: 'data-analyst',
    registry: 'registry.example.com',
    versions: [
      { version: '0.9.0', spec: { model: 'claude-3' }, published_at: '2025-03-01T00:00:00Z' },
    ],
  },
];

export const mockDeployments: DeploymentsListResponse = {
  deployments: [
    {
      name: 'code-reviewer',
      version: '1.0.0',
      status: 'Running',
      replicas: 1,
      ready: 1,
      created_at: '2025-04-01T00:00:00Z',
      components: ['deployment', 'service'],
    },
  ],
  count: 1,
  namespace: 'user-abc123',
};

export const handlers = [
  // GET /api/v1/agents
  http.get('/api/v1/agents', () => {
    return HttpResponse.json<AgentsListResponse>({
      agents: mockAgents,
      count: mockAgents.length,
    });
  }),

  // GET /api/v1/agents/:name
  http.get('/api/v1/agents/:name', ({ params }) => {
    const agent = mockAgents.find((a) => a.name === params.name);
    if (!agent) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(agent);
  }),

  // GET /api/v1/agents/:name/:version
  http.get('/api/v1/agents/:name/:version', ({ params }) => {
    const agent = mockAgents.find((a) => a.name === params.name);
    const version = agent?.versions.find((v) => v.version === params.version);
    if (!version) {
      return HttpResponse.json({ error: 'not_found' }, { status: 404 });
    }
    return HttpResponse.json(version);
  }),

  // GET /api/v1/deployments
  http.get('/api/v1/deployments', () => {
    return HttpResponse.json(mockDeployments);
  }),

  // POST /api/v1/deploy
  http.post('/api/v1/deploy', async ({ request }) => {
    const body = (await request.json()) as { name: string; version: string };
    return HttpResponse.json<DeployResponse>({
      status: 'deployed',
      name: body.name,
      version: body.version,
      k8s_namespace: 'user-abc123',
      deployed_at: new Date().toISOString(),
      resources: [{ kind: 'Deployment', name: body.name, status: 'created' }],
    });
  }),

  // POST /api/v1/undeploy
  http.post('/api/v1/undeploy', async ({ request }) => {
    const body = (await request.json()) as { name: string; version: string };
    return HttpResponse.json<UndeployResponse>({
      status: 'undeployed',
      name: body.name,
      version: body.version,
      k8s_namespace: 'user-abc123',
      undeployed_at: new Date().toISOString(),
      resources: [{ kind: 'Deployment', name: body.name, status: 'deleted' }],
    });
  }),
];
